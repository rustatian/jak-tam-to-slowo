package api

import (
	"context"
	"encoding/json/v2"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
)

// createDraft stores a queued draft job of the signed-in user. The user must have the AI consent. The stored input
// is the body with the trimmed texts and without unknown members.
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
	return jsonResponse(http.StatusAccepted, draft)
}
