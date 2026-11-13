package api

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../../api/openapi.yaml

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
)

// Server serves the API Gateway HTTP API routes of the API Lambda.
type Server struct {
	DB *pgxpool.Pool
	// EmailOf returns the email of a Cognito user; the access token has no email claim.
	EmailOf func(ctx context.Context, sub string) (string, error)
	// Send sends an email. An error means that the email service did not accept the message.
	Send func(ctx context.Context, m Mail) error
}

// Handle picks the route by its route key.
func (s *Server) Handle(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	switch req.RouteKey {
	case "GET /v1/me":
		return s.getMe(ctx, req), nil
	case "PATCH /v1/me":
		return s.patchMe(ctx, req), nil
	case "PUT /v1/me/ai-consent":
		return s.putConsent(ctx, req), nil
	case "DELETE /v1/me/ai-consent":
		return s.deleteConsent(ctx, req), nil
	case "POST /v1/groups":
		return s.createGroup(ctx, req), nil
	case "GET /v1/groups/{groupId}":
		return s.getGroup(ctx, req), nil
	case "PATCH /v1/groups/{groupId}":
		return s.patchGroup(ctx, req), nil
	case "GET /v1/groups/{groupId}/members":
		return s.listMembers(ctx, req), nil
	case "DELETE /v1/groups/{groupId}/members/{userId}":
		return s.removeMember(ctx, req), nil
	case "POST /v1/groups/{groupId}/invitations":
		return s.createInvitation(ctx, req), nil
	case "GET /v1/groups/{groupId}/invitations":
		return s.listInvitations(ctx, req), nil
	case "DELETE /v1/groups/{groupId}/invitations/{invitationId}":
		return s.cancelInvitation(ctx, req), nil
	case "GET /v1/invitations/{token}":
		return s.previewInvitation(ctx, req), nil
	case "POST /v1/invitations/{token}/acceptance":
		return s.acceptInvitation(ctx, req), nil
	case "POST /v1/groups/{groupId}/cards":
		return s.createCard(ctx, req), nil
	case "GET /v1/groups/{groupId}/cards":
		return s.listCards(ctx, req), nil
	case "PATCH /v1/groups/{groupId}/cards/{cardId}":
		return s.patchCard(ctx, req), nil
	case "DELETE /v1/groups/{groupId}/cards/{cardId}":
		return s.archiveCard(ctx, req), nil
	case "GET /v1/groups/{groupId}/study":
		return s.getStudyQueue(ctx, req), nil
	default:
		return jsonResponse(http.StatusNotFound, Error{Message: "not found"}), nil
	}
}

func subject(req events.APIGatewayV2HTTPRequest) string {
	if a := req.RequestContext.Authorizer; a != nil && a.JWT != nil {
		return a.JWT.Claims["sub"]
	}
	return ""
}

func (s *Server) getMe(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub := subject(req)
	if sub == "" {
		return jsonResponse(http.StatusUnauthorized, Error{Message: "unauthorized"})
	}
	me, err := s.me(ctx, sub)
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	return jsonResponse(http.StatusOK, me)
}

// serverError logs the cause and answers 500 with a fixed message.
func serverError(ctx context.Context, req events.APIGatewayV2HTTPRequest, sub string, err error) events.APIGatewayV2HTTPResponse {
	slog.ErrorContext(ctx, req.RouteKey, "sub", sub, "error", err)
	return jsonResponse(http.StatusInternalServerError, Error{Message: "internal error"})
}

// me reads the user, and makes the row on the first call.
func (s *Server) me(ctx context.Context, sub string) (Me, error) {
	me, found, err := readUser(ctx, s.DB, sub)
	if err != nil || found {
		return me, err
	}
	email, err := s.EmailOf(ctx, sub)
	if err != nil {
		return Me{}, fmt.Errorf("email: %w", err)
	}
	// A concurrent first call makes one transaction fail with 40001; its retry reads the other call's row.
	err = db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO users (id, email) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING", sub, email); err != nil {
			return err
		}
		me, _, err = readUser(ctx, tx, sub)
		return err
	})
	return me, err
}

func readUser(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, sub string) (Me, bool, error) {
	var me Me
	err := q.QueryRow(ctx, "SELECT id, email, display_name, direction, COALESCE(reminder, false), ai_consent_at, group_id FROM users WHERE id = $1", sub).
		Scan(&me.Id, &me.Email, &me.DisplayName, &me.Direction, &me.Reminder, &me.AiConsentAt, &me.GroupId)
	if errors.Is(err, pgx.ErrNoRows) {
		return Me{}, false, nil
	}
	return me, err == nil, err
}

// jsonResponse marshals v; the generated contract types always marshal.
func jsonResponse(status int, v any) events.APIGatewayV2HTTPResponse {
	body, _ := json.Marshal(v)
	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(body),
	}
}

func noContent() events.APIGatewayV2HTTPResponse {
	return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusNoContent}
}
