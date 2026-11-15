package api

import (
	"cmp"
	"context"
	_ "embed"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
)

// createDraft stores a queued draft job of the signed-in user and starts the job function for it. The user must have
// the AI consent. The stored input is the body with the trimmed texts and without unknown members. A job that did not
// start stays queued.
func (s *Server) createDraft(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub, fail := s.member(ctx, req)
	if fail != nil {
		return *fail
	}
	me, _, err := readUser(ctx, s.DB, sub)
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	if me.AiConsentAt == nil {
		return jsonResponse(http.StatusForbidden, Error{Message: "AI consent is required"})
	}
	var body DraftCreate
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return jsonResponse(http.StatusBadRequest, Error{Message: "invalid JSON body"})
	}
	if !checkText(&body.Text, 200) {
		return jsonResponse(http.StatusBadRequest, Error{Message: "text must have 1 to 200 characters"})
	}
	if !checkText(body.MeaningRu, 200) {
		return jsonResponse(http.StatusBadRequest, Error{Message: "meaningRu must have 1 to 200 characters"})
	}
	draft := Draft{Status: DraftStatusQueued}
	err = db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "INSERT INTO jobs (user_id, kind, input) VALUES ($1, 'draft', $2) RETURNING id", sub, body).Scan(&draft.Id)
	})
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	// The start is after the commit: db.Tx can run its function again, and the job function reads the committed row.
	if err := s.StartJob(ctx, sub, draft.Id); err != nil {
		slog.ErrorContext(ctx, req.RouteKey, "sub", sub, "error", err)
		return jsonResponse(http.StatusBadGateway, Error{Message: "the draft job did not start"})
	}
	return jsonResponse(http.StatusAccepted, draft)
}

// getDraft returns the state of a draft job of the signed-in user. It writes nothing.
func (s *Server) getDraft(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub, fail := s.member(ctx, req)
	if fail != nil {
		return *fail
	}
	// A job that did not end in 120 seconds reads as failed with unavailable, by the clock of the database.
	const late = "status IN ('queued', 'running') AND created_at < now() - interval '120 seconds'"
	draft := Draft{Id: req.PathParameters["draftId"]}
	// The ID is compared as text: PostgreSQL refuses a uuid parameter that is not a UUID (SQLSTATE 22P02).
	// https://www.postgresql.org/docs/16/errcodes-appendix.html
	err := s.DB.QueryRow(ctx, "SELECT CASE WHEN "+late+" THEN 'failed' ELSE status END, CASE WHEN "+late+" THEN 'unavailable' ELSE error END, "+
		"result->'card', result->'choices' FROM jobs WHERE user_id = $1 AND id::text = $2 AND kind = 'draft'", sub, draft.Id).
		Scan(&draft.Status, &draft.Error, &draft.Card, &draft.Choices)
	if errors.Is(err, pgx.ErrNoRows) {
		return jsonResponse(http.StatusNotFound, Error{Message: "draft not found"})
	}
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	return jsonResponse(http.StatusOK, draft)
}

// choicesForm and draftForm are the two forms of the result of a draft job: the meanings of the text, or a draft of
// a card. They are JSON text because the model writes the properties of its answer in the order of the schema, and
// the encoder writes a Go map with sorted keys. kind is first, so the model names the form before it writes the
// content. The card has its properties in the order of the prompt.
// https://platform.claude.com/docs/en/build-with-claude/structured-outputs#property-ordering
const (
	choicesForm = `{"type":"object","properties":{"kind":{"type":"string","enum":["choices"]},"choices":{"type":"array","items":{"type":"string"}}},"required":["kind","choices"],"additionalProperties":false}`
	draftForm   = `{"type":"object","properties":{"kind":{"type":"string","enum":["draft"]},"card":{"type":"object","properties":{"pl":{"type":"string"},"ru":{"type":"string"},"examplePl":{"type":"string"},"exampleRu":{"type":"string"},"meaningRu":{"type":"string"},"alternativesPl":{"type":"array","items":{"type":"string"},"minItems":1},"alternativesRu":{"type":"array","items":{"type":"string"}}},"required":["pl","ru","examplePl","exampleRu","meaningRu","alternativesPl","alternativesRu"],"additionalProperties":false}},"required":["kind","card"],"additionalProperties":false}`
)

// draftSchema is the JSON schema of the answer of a draft job for the input. An input with a picked meaning allows
// only a draft: in some calls the model answers with meanings again when the schema allows them.
// https://platform.claude.com/docs/en/build-with-claude/structured-outputs
func draftSchema(input DraftCreate) map[string]any {
	result := `{"anyOf":[` + choicesForm + `,` + draftForm + `]}`
	if input.MeaningRu != nil {
		result = draftForm
	}
	return map[string]any{"type": "object", "properties": map[string]any{"result": jsontext.Value(result)}, "required": []any{"result"}, "additionalProperties": false}
}

// draftPrompt is the system prompt of a draft job.
//
//go:embed draft_prompt.txt
var draftPrompt string

// JobEvent is the event of the job function: a stored job of a user.
type JobEvent struct {
	UserID string `json:"userId"`
	JobID  string `json:"jobId"`
}

// claimJob sets a queued job of the user to running, reports whether it did, and returns the kind and the input of
// the job. A job of 60 seconds or more stays queued, so no job ends after the poll shows it as failed (at 120 seconds
// for a draft, at 320 seconds for a grammar check).
func (s *Server) claimJob(ctx context.Context, userID, jobID string) (kind string, input []byte, claimed bool, err error) {
	err = db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		// Each run sets kind, input and claimed: on Aurora DSQL a second claim fails at the commit, and db.Tx runs this
		// function again.
		kind, input, claimed = "", nil, false
		// The ID is compared as text: PostgreSQL refuses a uuid parameter that is not a UUID (SQLSTATE 22P02).
		// https://www.postgresql.org/docs/16/errcodes-appendix.html
		err := tx.QueryRow(ctx, "UPDATE jobs SET status = 'running' WHERE user_id = $1 AND id::text = $2 AND status = 'queued' AND created_at > now() - interval '60 seconds' RETURNING kind, input",
			userID, jobID).Scan(&kind, &input)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		claimed = err == nil
		return err
	})
	return kind, input, claimed, err
}

// RunJob claims the job of the event and runs it by its kind: a grammar check for the kind grammar, and a draft for
// the kind draft. A job that it does not claim stays as it is. It returns an error when a database call fails, and
// when the API answers with a status that needs the operator.
func (s *Server) RunJob(ctx context.Context, event JobEvent) error {
	kind, input, claimed, err := s.claimJob(ctx, event.UserID, event.JobID)
	if err != nil || !claimed {
		return err
	}
	if kind == "grammar" {
		return s.runGrammarCheck(ctx, event, input)
	}
	return s.runDraft(ctx, event, input)
}

// runDraft asks the AI model for a draft for the input of a claimed draft job, and ends the job as done with the
// checked answer or as failed with the cause.
func (s *Server) runDraft(ctx context.Context, event JobEvent, input []byte) error {
	var body DraftCreate
	if err := json.Unmarshal(input, &body); err != nil {
		return err
	}
	// The input of a stored job always marshals.
	text, _ := json.Marshal(body)
	callCtx, cancel := context.WithTimeout(ctx, 50*time.Second)
	start := time.Now()
	resp, callErr := s.AI(callCtx, AIRequest{Model: "claude-opus-5-5", Effort: "high", MaxTokens: 16000, System: draftPrompt, Schema: draftSchema(body), Input: string(text)})
	duration := time.Since(start)
	cancel()
	result, fail, cause := draftOutcome(body, resp, callErr)
	err := s.endJob(ctx, event, result, string(fail))
	logCall(ctx, "draft job", event, string(fail), cause, resp, duration, callErr)
	if err != nil {
		return err
	}
	if needsOperator(callErr) {
		return callErr
	}
	return nil
}

// endJob ends the running job of the event as failed with fail, or as done with result when fail is empty. ctx is the
// context of the event, because a call can use all the time of its own context.
func (s *Server) endJob(ctx context.Context, event JobEvent, result map[string]any, fail string) error {
	return db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		const running = " WHERE user_id = $1 AND id::text = $2 AND status = 'running'"
		if fail != "" {
			_, err := tx.Exec(ctx, "UPDATE jobs SET status = 'failed', error = $3"+running, event.UserID, event.JobID, fail)
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE jobs SET status = 'done', result = $3"+running, event.UserID, event.JobID, result)
		return err
	})
}

// logCall writes the log line msg for one call of the AI model in the job of the event, with the pairs of more after
// attempts. The line has no text of the learner and no text of the answer: cause names the broken rule or gives counts.
func logCall(ctx context.Context, msg string, event JobEvent, fail, cause string, resp AIResponse, duration time.Duration, callErr error, more ...any) {
	level, args := slog.LevelInfo, append([]any{"jobId", event.JobID, "userId", event.UserID, "outcome", cmp.Or(fail, "done"), "cause", cause, "model", resp.Model,
		"stopReason", resp.StopReason, "duration", duration, "requestId", resp.RequestID, "inferenceGeo", resp.InferenceGeo, "attempts", resp.Attempts}, more...)
	if callErr != nil {
		level, args = slog.LevelError, append(args, "error", callErr)
	}
	slog.Log(ctx, level, msg, args...)
}

// needsOperator reports that the API answered the call with a status of 400 to 404: a bad request, the spend limit,
// the API key, the billing, the permission or the model. The error of the function makes its alarm send an email.
// https://platform.claude.com/docs/en/api/errors#http-errors
func needsOperator(callErr error) bool {
	apiErr, ok := errors.AsType[*AIError](callErr)
	return ok && apiErr.Status >= 400 && apiErr.Status <= 404
}

// draftOutcome checks the answer of the call for the input of a draft job. fail is the cause of a failed job; when it
// is empty, the job is done with result. cause tells the log what an invalid answer broke: the rule of a card, or the
// counts of a list of meanings.
// https://platform.claude.com/docs/en/build-with-claude/handling-stop-reasons
func draftOutcome(input DraftCreate, resp AIResponse, callErr error) (result map[string]any, fail DraftError, cause string) {
	switch {
	case callErr != nil:
		return nil, DraftErrorUnavailable, ""
	// A refusal can come with a text, so the stop reason is read first.
	case resp.StopReason == "refusal":
		return nil, DraftErrorRefused, ""
	case resp.StopReason != "end_turn":
		return nil, DraftErrorInvalid, "stop reason"
	}
	var answer struct {
		Result struct {
			Kind    string      `json:"kind"`
			Card    *CardUpdate `json:"card"`
			Choices []string    `json:"choices"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &answer, json.RejectUnknownMembers(true)); err != nil {
		return nil, DraftErrorInvalid, "not the JSON of the schema"
	}
	r := answer.Result
	switch {
	case strings.EqualFold(r.Kind, "draft") && r.Card != nil && r.Choices == nil:
		c := r.Card
		// checkCard lets a nil field pass, and a card needs each of the seven fields.
		if c.Pl == nil || c.Ru == nil || c.ExamplePl == nil || c.ExampleRu == nil || c.MeaningRu == nil || c.AlternativesPl == nil || c.AlternativesRu == nil {
			return nil, DraftErrorInvalid, "a card field is missing"
		}
		if msg := checkCard(c); msg != "" {
			return nil, DraftErrorInvalid, msg
		}
		return map[string]any{"card": c}, "", ""
	// Meanings are only for a text without a picked meaning. checkTerms trims each meaning and allows at most 8. The
	// sorted list without repeats is shorter when two trimmed meanings are equal.
	case strings.EqualFold(r.Kind, "choices") && r.Card == nil && input.MeaningRu == nil && len(r.Choices) >= 2 && checkTerms(&r.Choices) &&
		len(slices.Compact(slices.Sorted(slices.Values(r.Choices)))) == len(r.Choices):
		return map[string]any{"choices": r.Choices}, "", ""
	}
	invalid := 0
	terms := make([]string, len(r.Choices))
	for i, choice := range r.Choices {
		var ok bool
		if terms[i], ok = trimmedText(choice, 200); !ok {
			invalid++
		}
	}
	slices.Sort(terms)
	return nil, DraftErrorInvalid, fmt.Sprintf("%d choices, %d distinct, %d invalid, card %t, meaning %t",
		len(terms), len(slices.Compact(terms)), invalid, r.Card != nil, input.MeaningRu != nil)
}
