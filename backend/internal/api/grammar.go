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
