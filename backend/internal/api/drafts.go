package api

import (
	"context"
	"encoding/json/v2"
	"errors"
	"log/slog"
	"net/http"

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
	draft := Draft{Status: Queued}
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

// JobEvent is the event of the job function: a stored job of a user.
type JobEvent struct {
	UserID string `json:"userId"`
	JobID  string `json:"jobId"`
}

// claimJob sets a queued job of the user to running and reports whether it did. A job of 60 seconds or more stays
// queued, so no job ends after the poll shows it as failed at 120 seconds.
func (s *Server) claimJob(ctx context.Context, userID, jobID string) (claimed bool, err error) {
	err = db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		// The ID is compared as text: PostgreSQL refuses a uuid parameter that is not a UUID (SQLSTATE 22P02).
		// https://www.postgresql.org/docs/16/errcodes-appendix.html
		tag, err := tx.Exec(ctx, "UPDATE jobs SET status = 'running' WHERE user_id = $1 AND id::text = $2 AND status = 'queued' AND created_at > now() - interval '60 seconds'",
			userID, jobID)
		// Each run sets claimed: on Aurora DSQL a second claim fails at the commit, and db.Tx runs this function again.
		claimed = tag.RowsAffected() == 1
		return err
	})
	return claimed, err
}

// RunJob claims the job of the event and ends it as failed with the error unavailable. A job that it does not claim
// stays as it is. It returns an error only when a database call fails.
func (s *Server) RunJob(ctx context.Context, event JobEvent) error {
	claimed, err := s.claimJob(ctx, event.UserID, event.JobID)
	if err != nil || !claimed {
		return err
	}
	return db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE jobs SET status = 'failed', error = 'unavailable' WHERE user_id = $1 AND id::text = $2 AND status = 'running'",
			event.UserID, event.JobID)
		return err
	})
}
