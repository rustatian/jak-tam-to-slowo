package api

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
)

// trimmedName removes the spaces at the ends of a name. ok reports that the result has 1 to 40 characters.
func trimmedName(s string) (name string, ok bool) {
	name = strings.TrimSpace(s)
	n := utf8.RuneCountInString(name)
	return name, n >= 1 && n <= 40
}

// patchMe changes the profile fields that the body names. A field that the body leaves out keeps its value.
func (s *Server) patchMe(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub := subject(req)
	if sub == "" {
		return jsonResponse(http.StatusUnauthorized, Error{Message: "unauthorized"})
	}
	var update MeUpdate
	if err := json.Unmarshal([]byte(req.Body), &update); err != nil {
		return jsonResponse(http.StatusBadRequest, Error{Message: "invalid JSON body"})
	}
	if update.DisplayName != nil {
		name, ok := trimmedName(*update.DisplayName)
		if !ok {
			return jsonResponse(http.StatusBadRequest, Error{Message: "displayName must have 1 to 40 characters"})
		}
		update.DisplayName = &name
	}
	if update.Direction != nil && !update.Direction.Valid() {
		return jsonResponse(http.StatusBadRequest, Error{Message: "direction must be ru-pl or pl-ru"})
	}
	// The first call of a new user can be this one, so the row is made first.
	if _, err := s.me(ctx, sub); err != nil {
		return serverError(ctx, req, sub, err)
	}
	var me Me
	err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			"UPDATE users SET display_name = COALESCE($2, display_name), direction = COALESCE($3, direction), reminder = COALESCE($4, reminder) WHERE id = $1",
			sub, update.DisplayName, (*string)(update.Direction), update.Reminder); err != nil {
			return err
		}
		var err error
		me, _, err = readUser(ctx, tx, sub)
		return err
	})
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	return jsonResponse(http.StatusOK, me)
}

// putConsent stores the AI consent time. A second call keeps the first time.
func (s *Server) putConsent(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub := subject(req)
	if sub == "" {
		return jsonResponse(http.StatusUnauthorized, Error{Message: "unauthorized"})
	}
	if _, err := s.me(ctx, sub); err != nil {
		return serverError(ctx, req, sub, err)
	}
	err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE users SET ai_consent_at = COALESCE(ai_consent_at, now()) WHERE id = $1", sub)
		return err
	})
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	return noContent()
}

// deleteConsent removes the AI consent. A missing row has no consent, so it needs no row.
func (s *Server) deleteConsent(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub := subject(req)
	if sub == "" {
		return jsonResponse(http.StatusUnauthorized, Error{Message: "unauthorized"})
	}
	err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE users SET ai_consent_at = NULL WHERE id = $1", sub)
		return err
	})
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	return noContent()
}
