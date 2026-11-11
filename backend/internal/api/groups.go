package api

import (
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
)

var errInGroup = errors.New("the user is in a group")

// member returns the ID of the signed-in user when the user is a member of the group in the path.
// Otherwise fail holds the answer. A handler passes the path's group ID to SQL only after this check:
// PostgreSQL refuses a uuid parameter that is not a UUID (SQLSTATE 22P02).
func (s *Server) member(ctx context.Context, req events.APIGatewayV2HTTPRequest) (sub string, fail *events.APIGatewayV2HTTPResponse) {
	sub = subject(req)
	if sub == "" {
		resp := jsonResponse(http.StatusUnauthorized, Error{Message: "unauthorized"})
		return "", &resp
	}
	me, _, err := readUser(ctx, s.DB, sub)
	if err != nil {
		resp := serverError(ctx, req, sub, err)
		return "", &resp
	}
	if me.GroupId == nil || *me.GroupId != req.PathParameters["groupId"] {
		resp := jsonResponse(http.StatusForbidden, Error{Message: "not a member of this group"})
		return "", &resp
	}
	return sub, nil
}

// createGroup makes a group with the signed-in user as its first member.
func (s *Server) createGroup(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub := subject(req)
	if sub == "" {
		return jsonResponse(http.StatusUnauthorized, Error{Message: "unauthorized"})
	}
	var body GroupCreate
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return jsonResponse(http.StatusBadRequest, Error{Message: "invalid JSON body"})
	}
	name, ok := trimmedName(body.Name)
	if !ok {
		return jsonResponse(http.StatusBadRequest, Error{Message: "name must have 1 to 40 characters"})
	}
	// The first call of a new user can be this one, so the row is made first.
	if _, err := s.me(ctx, sub); err != nil {
		return serverError(ctx, req, sub, err)
	}
	var group Group
	err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "INSERT INTO groups (name) VALUES ($1) RETURNING id, name, created_at", name).
			Scan(&group.Id, &group.Name, &group.CreatedAt); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "UPDATE users SET group_id = $1 WHERE id = $2 AND group_id IS NULL", group.Id, sub)
		if err != nil {
			return err
		}
		// The error ends the transaction, so the new group row does not stay.
		if tag.RowsAffected() == 0 {
			return errInGroup
		}
		return nil
	})
	if errors.Is(err, errInGroup) {
		return jsonResponse(http.StatusConflict, Error{Message: "already in a group"})
	}
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	return jsonResponse(http.StatusCreated, group)
}

func (s *Server) getGroup(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub, fail := s.member(ctx, req)
	if fail != nil {
		return *fail
	}
	var group Group
	if err := s.DB.QueryRow(ctx, "SELECT id, name, created_at FROM groups WHERE id = $1", req.PathParameters["groupId"]).
		Scan(&group.Id, &group.Name, &group.CreatedAt); err != nil {
		return serverError(ctx, req, sub, err)
	}
	return jsonResponse(http.StatusOK, group)
}

// patchGroup changes the group fields that the body names. A field that the body leaves out keeps its value.
func (s *Server) patchGroup(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub, fail := s.member(ctx, req)
	if fail != nil {
		return *fail
	}
	var update GroupUpdate
	if err := json.Unmarshal([]byte(req.Body), &update); err != nil {
		return jsonResponse(http.StatusBadRequest, Error{Message: "invalid JSON body"})
	}
	if update.Name != nil {
		name, ok := trimmedName(*update.Name)
		if !ok {
			return jsonResponse(http.StatusBadRequest, Error{Message: "name must have 1 to 40 characters"})
		}
		update.Name = &name
	}
	var group Group
	err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "UPDATE groups SET name = COALESCE($2, name) WHERE id = $1 RETURNING id, name, created_at", req.PathParameters["groupId"], update.Name).
			Scan(&group.Id, &group.Name, &group.CreatedAt)
	})
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	return jsonResponse(http.StatusOK, group)
}
