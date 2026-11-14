package api

import (
	"context"
	_ "embed"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
)

var (
	errNotCurrent = errors.New("the progress version is not current")
	errOtherCard  = errors.New("the attempt ID is stored for another card")
)

// createReview grades an answer of the signed-in learner to a translation card of the group in the path, and counts
// it in the progress row of the learner for the card.
func (s *Server) createReview(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub, fail := s.member(ctx, req)
	if fail != nil {
		return *fail
	}
	var body ReviewCreate
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return jsonResponse(http.StatusBadRequest, Error{Message: "invalid JSON body"})
	}
	var attempt pgtype.UUID
	if err := attempt.Scan(body.AttemptId); err != nil {
		return jsonResponse(http.StatusBadRequest, Error{Message: "attemptId must be a UUID"})
	}
	switch {
	case body.ProgressVersion < 0:
		return jsonResponse(http.StatusBadRequest, Error{Message: "progressVersion must be 0 or more"})
	case !body.Direction.Valid():
		return jsonResponse(http.StatusBadRequest, Error{Message: "direction must be ru-pl or pl-ru"})
	case utf8.RuneCountInString(body.Answer) > 500:
		return jsonResponse(http.StatusBadRequest, Error{Message: "answer must have at most 500 characters"})
	}
	now := time.Now()
	var review Review
	var status int
	// The reads and the writes use one transaction. When another request writes the progress row after the reads,
	// the transaction fails with SQLSTATE 40001 and db.Tx runs the function again. The second run answers 200 when
	// the other request stored the same attempt, and 409 when it had another attempt. So each run sets review and
	// status.
	err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		// A stored attempt gets its stored grade and counts nothing. The card ID is compared as text, as in the read
		// below.
		var stored Grade
		var sameCard bool
		err := tx.QueryRow(ctx, "SELECT grade, card_id::text = $3 FROM reviews WHERE user_id = $1 AND attempt_id = $2",
			sub, attempt, body.CardId).Scan(&stored, &sameCard)
		switch {
		case err == nil && !sameCard:
			return errOtherCard
		case err == nil:
			card, err := scanCard(tx.QueryRow(ctx, "SELECT "+cardColumns+" FROM cards WHERE group_id = $1 AND id::text = $2",
				req.PathParameters["groupId"], body.CardId))
			if err != nil {
				return err
			}
			review = Review{Grade: stored, Correction: grade(card, body.Direction, body.Answer).Correction}
			status = http.StatusOK
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		var version int
		// The ID is compared as text: PostgreSQL refuses a uuid parameter that is not a UUID (SQLSTATE 22P02).
		// https://www.postgresql.org/docs/16/errcodes-appendix.html
		// A card without a progress row of the learner has the version 0.
		card, err := scanCard(tx.QueryRow(ctx, "SELECT "+cardColumns+", COALESCE(version, 0) "+
			"FROM cards LEFT JOIN card_progress ON card_progress.card_id = cards.id AND card_progress.user_id = $3 "+
			"WHERE group_id = $1 AND cards.id::text = $2 AND NOT archived AND kind = 'translation'",
			req.PathParameters["groupId"], body.CardId, sub), &version)
		if err != nil {
			return err
		}
		if version != body.ProgressVersion {
			return errNotCurrent
		}
		review = grade(card, body.Direction, body.Answer)
		status = http.StatusCreated
		// add is the counts and the time of this answer. Each grade that is not correct counts as a miss.
		add := progress{incorrect: 1, lastIncorrectAt: &now}
		if review.Grade == Correct {
			add = progress{correct: 1, lastCorrectAt: &now}
		}
		// EXCLUDED is the row proposed for insertion, so the update adds the counts of this answer and keeps the time
		// that this answer does not set.
		// https://www.postgresql.org/docs/16/sql-insert.html#SQL-ON-CONFLICT
		_, err = tx.Exec(ctx, "INSERT INTO card_progress (user_id, card_id, correct, incorrect, last_correct_at, last_incorrect_at, version) "+
			"VALUES ($1, $2, $3, $4, $5, $6, 1) ON CONFLICT (card_id, user_id) DO UPDATE SET "+
			"correct = card_progress.correct + EXCLUDED.correct, incorrect = card_progress.incorrect + EXCLUDED.incorrect, "+
			"last_correct_at = COALESCE(EXCLUDED.last_correct_at, card_progress.last_correct_at), "+
			"last_incorrect_at = COALESCE(EXCLUDED.last_incorrect_at, card_progress.last_incorrect_at), "+
			"version = card_progress.version + 1",
			sub, card.Id, add.correct, add.incorrect, add.lastCorrectAt, add.lastIncorrectAt)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO reviews (user_id, attempt_id, card_id, grade) VALUES ($1, $2, $3, $4)",
			sub, attempt, card.Id, review.Grade)
		return err
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return jsonResponse(http.StatusNotFound, Error{Message: "card not found"})
	case errors.Is(err, errNotCurrent):
		return jsonResponse(http.StatusConflict, Error{Message: "progressVersion is not current"})
	case errors.Is(err, errOtherCard):
		return jsonResponse(http.StatusConflict, Error{Message: "attemptId is used for another card"})
	case err != nil:
		return serverError(ctx, req, sub, err)
	}
	return jsonResponse(status, review)
}

// grade grades an answer to the card for the direction of the prompt. Only the terms of the answer side count.
func grade(card Card, direction Direction, answer string) Review {
	polish := direction == RuPl
	terms := append([]string{card.Ru}, card.AlternativesRu...)
	if polish {
		terms = append([]string{card.Pl}, card.AlternativesPl...)
	}
	key := answerKey(answer)
	if slices.ContainsFunc(terms, func(term string) bool { return answerKey(term) == key }) {
		return Review{Grade: Correct, Correction: terms[0]}
	}
	// A learner without a Polish keyboard types the nine Polish letters without their marks.
	if polish {
		key = searchKey(answer)
		if i := slices.IndexFunc(terms, func(term string) bool { return searchKey(term) == key }); i >= 0 {
			return Review{Grade: Almost, Correction: terms[i]}
		}
	}
	return Review{Grade: Again, Correction: terms[0]}
}

// gradePrompt is the system prompt of a grade of the AI model: the prompt file of the old app, byte for byte.
//
//go:embed grade_prompt.txt
var gradePrompt string

// gradeProperties is JSON text, because the model writes the properties of its answer in the order of the schema, and
// the encoder writes a Go map with sorted keys. The order is the order of the prompt.
// https://platform.claude.com/docs/en/build-with-claude/structured-outputs#property-ordering
const gradeProperties = `{"grade":{"type":"string","enum":["correct","almost","again"]},"explanation":{"type":"string"}}`

// gradeSchema is the JSON schema of the answer of a grade of the AI model.
var gradeSchema = map[string]any{"type": "object", "properties": jsontext.Value(gradeProperties), "required": []any{"grade", "explanation"}, "additionalProperties": false}

// aiGrade is the grade of the AI model for one answer.
type aiGrade struct {
	grade       Grade
	explanation string
}

// gradeOutcome checks the answer of the call for a grade. cause names the first broken rule for the log, and it is
// empty for a valid answer.
// https://platform.claude.com/docs/en/build-with-claude/handling-stop-reasons
func gradeOutcome(resp AIResponse, callErr error) (g aiGrade, cause string) {
	switch {
	case callErr != nil:
		return aiGrade{}, "call"
	// A refusal can come with a text, so the stop reason is read first.
	case resp.StopReason == "refusal":
		return aiGrade{}, "refusal"
	case resp.StopReason != "end_turn":
		return aiGrade{}, "stop reason"
	}
	var answer struct {
		Grade       string `json:"grade"`
		Explanation string `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &answer); err != nil {
		return aiGrade{}, "not the JSON of the schema"
	}
	// Structured outputs do not guarantee the capital letters of an enum value.
	// https://platform.claude.com/docs/en/build-with-claude/structured-outputs#invalid-outputs
	g = aiGrade{grade: Grade(strings.ToLower(answer.Grade)), explanation: strings.TrimSpace(answer.Explanation)}
	if !g.grade.Valid() {
		return aiGrade{}, "grade"
	}
	if n := utf8.RuneCountInString(g.explanation); n < 1 || n > 500 {
		return aiGrade{}, "explanation"
	}
	return g, ""
}

// gradeWithAI asks the AI model for a grade of the answer to the card for the direction, and writes one log line. ok
// is false when the call failed or the answer broke a rule of gradeOutcome.
func (s *Server) gradeWithAI(ctx context.Context, sub string, card Card, direction Direction, answer string) (g aiGrade, ok bool) {
	// The members have the names and the order of the input of the old app.
	var input struct {
		Card struct {
			Pl             string   `json:"pl"`
			Ru             string   `json:"ru"`
			ExamplePl      string   `json:"examplePl"`
			ExampleRu      string   `json:"exampleRu"`
			MeaningRu      string   `json:"meaningRu"`
			AlternativesPl []string `json:"alternativesPl"`
			AlternativesRu []string `json:"alternativesRu"`
		} `json:"card"`
		Direction Direction `json:"direction"`
		Answer    string    `json:"answer"`
	}
	c := &input.Card
	c.Pl, c.Ru, c.ExamplePl, c.ExampleRu, c.MeaningRu, c.AlternativesPl, c.AlternativesRu = card.Pl, card.Ru, card.ExamplePl, card.ExampleRu, card.MeaningRu, card.AlternativesPl, card.AlternativesRu
	input.Direction, input.Answer = direction, answer
	// The texts of a stored card and of a decoded body are valid UTF-8, so the input always marshals.
	text, _ := json.Marshal(input)
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	start := time.Now()
	resp, callErr := s.AI(callCtx, AIRequest{Model: "claude-sonnet-5-5", Effort: "low", MaxTokens: 16000, System: gradePrompt, Schema: gradeSchema, Input: string(text)})
	duration := time.Since(start)
	cancel()
	g, cause := gradeOutcome(resp, callErr)
	// The line has no text of the card, of the answer or of the explanation.
	level, args := slog.LevelInfo, []any{"sub", sub, "cause", cause, "model", resp.Model, "stopReason", resp.StopReason, "duration", duration,
		"requestId", resp.RequestID, "inferenceGeo", resp.InferenceGeo, "attempts", resp.Attempts}
	if callErr != nil {
		level, args = slog.LevelError, append(args, "error", callErr)
	}
	slog.Log(ctx, level, "review grade", args...)
	return g, cause == ""
}
