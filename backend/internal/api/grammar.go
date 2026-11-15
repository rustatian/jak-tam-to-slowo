package api

import (
	"context"
	_ "embed"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
)

// createGrammarCheck stores a queued grammar check of the signed-in user and starts the job function for it. The user
// must have the AI consent. The stored input is the body with the trimmed text and without unknown members. A check
// that did not start stays queued.
func (s *Server) createGrammarCheck(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
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
	var body GrammarCheckCreate
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return jsonResponse(http.StatusBadRequest, Error{Message: "invalid JSON body"})
	}
	if !checkText(&body.Text, 500) {
		return jsonResponse(http.StatusBadRequest, Error{Message: "text must have 1 to 500 characters"})
	}
	check := GrammarCheck{Status: GrammarCheckStatusQueued}
	err = db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "INSERT INTO jobs (user_id, kind, input) VALUES ($1, 'grammar', $2) RETURNING id", sub, body).Scan(&check.Id)
	})
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	// The start is after the commit: db.Tx can run its function again, and the job function reads the committed row.
	if err := s.StartJob(ctx, sub, check.Id); err != nil {
		slog.ErrorContext(ctx, req.RouteKey, "sub", sub, "error", err)
		return jsonResponse(http.StatusBadGateway, Error{Message: "the grammar check did not start"})
	}
	return jsonResponse(http.StatusAccepted, check)
}

// getGrammarCheck returns the state of a grammar check of the signed-in user. It writes nothing.
func (s *Server) getGrammarCheck(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub, fail := s.member(ctx, req)
	if fail != nil {
		return *fail
	}
	// A check that did not end in 320 seconds reads as failed with unavailable, by the clock of the database. The job
	// function claims a check only in its first 60 seconds and has 260 seconds after the claim.
	const late = "status IN ('queued', 'running') AND created_at < now() - interval '320 seconds'"
	check := GrammarCheck{Id: req.PathParameters["checkId"]}
	// The ID is compared as text: PostgreSQL refuses a uuid parameter that is not a UUID (SQLSTATE 22P02).
	// https://www.postgresql.org/docs/16/errcodes-appendix.html
	err := s.DB.QueryRow(ctx, "SELECT CASE WHEN "+late+" THEN 'failed' ELSE status END, CASE WHEN "+late+" THEN 'unavailable' ELSE error END, "+
		"result->'cards' FROM jobs WHERE user_id = $1 AND id::text = $2 AND kind = 'grammar'", sub, check.Id).
		Scan(&check.Status, &check.Error, &check.Cards)
	if errors.Is(err, pgx.ErrNoRows) {
		return jsonResponse(http.StatusNotFound, Error{Message: "grammar check not found"})
	}
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	return jsonResponse(http.StatusOK, check)
}

// grammarCards is the JSON schema of the cards in the answer of a grammar check. It is JSON text because the model
// writes the properties of its answer in the order of the schema, and the encoder writes a Go map with sorted keys.
// The card has its properties in the order of the prompt.
// https://platform.claude.com/docs/en/build-with-claude/structured-outputs#property-ordering
const grammarCards = `{"type":"array","items":{"type":"object","properties":{"topic":{"type":"string"},"used":{"type":"string"},"right":{"type":"string"},"sentence":{"type":"string"},"answer":{"type":"string"},"alternatives":{"type":"array","items":{"type":"string"}},"hint":{"type":"string"},"facts":{"type":"array","items":{"type":"string"}},"slots":{"type":"array","items":{"type":"string"}},"rule":{"type":"string"},"examples":{"type":"array","items":{"type":"string"},"minItems":1},"table":{"anyOf":[{"type":"object","properties":{"title":{"type":"string"},"columns":{"type":"array","items":{"type":"string"},"minItems":1},"groups":{"type":"array","items":{"type":"object","properties":{"title":{"type":["string","null"]},"rows":{"type":"array","items":{"type":"object","properties":{"names":{"type":"array","items":{"type":"string"},"minItems":1},"endings":{"type":"string"},"examples":{"type":"array","items":{"type":"string"},"minItems":1},"hit":{"type":"boolean"}},"required":["names","endings","examples","hit"],"additionalProperties":false},"minItems":1}},"required":["title","rows"],"additionalProperties":false},"minItems":1}},"required":["title","columns","groups"],"additionalProperties":false},{"type":"null"}]}},"required":["topic","used","right","sentence","answer","alternatives","hint","facts","slots","rule","examples","table"],"additionalProperties":false}}`

// grammarSchema is the JSON schema of the answer of a grammar check.
// https://platform.claude.com/docs/en/build-with-claude/structured-outputs
var grammarSchema = map[string]any{"type": "object", "properties": map[string]any{"cards": jsontext.Value(grammarCards)}, "required": []any{"cards"}, "additionalProperties": false}

// grammarPrompt is the system prompt of a grammar check.
//
//go:embed grammar_prompt.txt
var grammarPrompt string

// runGrammarCheck asks the AI model for the grammar cards of the input of a claimed grammar check, and ends the check
// as done with the checked cards or as failed with the cause. An invalid first answer gets a second call with the same
// request.
func (s *Server) runGrammarCheck(ctx context.Context, event JobEvent, input []byte) error {
	var body GrammarCheckCreate
	if err := json.Unmarshal(input, &body); err != nil {
		return err
	}
	// The input of a stored job always marshals.
	text, _ := json.Marshal(body)
	req := AIRequest{Model: "claude-opus-5-5", Effort: "xhigh", MaxTokens: 16000, System: grammarPrompt, Schema: grammarSchema, Input: string(text)}
	var (
		cards   []CardCreate
		fail    GrammarCheckError
		callErr error
	)
	for n := 1; n <= 2; n++ {
		callCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		start := time.Now()
		var resp AIResponse
		resp, callErr = s.AI(callCtx, req)
		duration := time.Since(start)
		cancel()
		var cause string
		cards, fail, cause = grammarOutcome(resp, callErr)
		logCall(ctx, "grammar check", event, string(fail), cause, resp, duration, callErr, "call", n)
		if fail != GrammarCheckErrorInvalid {
			break
		}
	}
	if err := s.endJob(ctx, event, map[string]any{"cards": cards}, string(fail)); err != nil {
		return err
	}
	if needsOperator(callErr) {
		return callErr
	}
	return nil
}

// grammarOutcome checks the answer of a call of a grammar check. fail is the cause of a failed check; when it is
// empty, the check is done with cards. cause tells the log what an invalid answer broke: the stop reason, the JSON,
// the count of the cards, or the rule of a card.
// https://platform.claude.com/docs/en/build-with-claude/handling-stop-reasons
func grammarOutcome(resp AIResponse, callErr error) (cards []CardCreate, fail GrammarCheckError, cause string) {
	switch {
	case callErr != nil:
		return nil, GrammarCheckErrorUnavailable, ""
	// A refusal can come with a text, so the stop reason is read first.
	case resp.StopReason == "refusal":
		return nil, GrammarCheckErrorRefused, ""
	case resp.StopReason != "end_turn":
		return nil, GrammarCheckErrorInvalid, "stop reason"
	}
	var answer struct {
		Cards []struct {
			Topic        string        `json:"topic"`
			Used         string        `json:"used"`
			Right        string        `json:"right"`
			Sentence     string        `json:"sentence"`
			Answer       string        `json:"answer"`
			Alternatives *[]string     `json:"alternatives"`
			Hint         string        `json:"hint"`
			Facts        []string      `json:"facts"`
			Slots        []string      `json:"slots"`
			Rule         string        `json:"rule"`
			Examples     []string      `json:"examples"`
			Table        *GrammarTable `json:"table"`
		} `json:"cards"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &answer, json.RejectUnknownMembers(true)); err != nil {
		return nil, GrammarCheckErrorInvalid, "not the JSON of the schema"
	}
	if len(answer.Cards) > 10 {
		return nil, GrammarCheckErrorInvalid, fmt.Sprintf("%d cards", len(answer.Cards))
	}
	cards = make([]CardCreate, len(answer.Cards))
	for i, a := range answer.Cards {
		cards[i] = CardCreate{Kind: new(CardKindGrammar), Pl: a.Answer, ExamplePl: a.Sentence, AlternativesPl: a.Alternatives, Grammar: &Grammar{
			Topic: a.Topic, Used: a.Used, Right: a.Right, Hint: a.Hint, Facts: a.Facts, Slots: a.Slots, Rule: a.Rule, Examples: a.Examples, Table: a.Table}}
		if msg := CheckGrammarCard(&cards[i]); msg != "" {
			return nil, GrammarCheckErrorInvalid, fmt.Sprintf("card %d: %s", i+1, msg)
		}
	}
	return cards, "", ""
}
