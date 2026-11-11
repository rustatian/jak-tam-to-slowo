package api

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"
	"slices"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
)

var (
	errInGroup   = errors.New("the user is in a group")
	errNotMember = errors.New("the user is not a member of the group")
)

// member returns the ID of the signed-in user when the user is a member of the group in the path.
// Otherwise fail holds the answer. A handler passes the path's group ID to SQL only after this check:
// PostgreSQL refuses a uuid parameter that is not a UUID (SQLSTATE 22P02, invalid_text_representation).
// https://www.postgresql.org/docs/16/errcodes-appendix.html
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

func (s *Server) listMembers(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub, fail := s.member(ctx, req)
	if fail != nil {
		return *fail
	}
	rows, err := s.DB.Query(ctx, "SELECT id, display_name FROM users WHERE group_id = $1", req.PathParameters["groupId"])
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	members, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Member, error) {
		var m Member
		err := row.Scan(&m.Id, &m.DisplayName)
		return m, err
	})
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	// Aurora DSQL has only the C collation, which sorts by bytes, so the order for readers is made here.
	// https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-postgresql-compatibility-migration-guide.html#working-with-postgresql-compatibility-unsupported-limitations
	// A Collator keeps the state of a comparison in itself, so each call makes its own.
	polish := collate.New(language.Polish)
	slices.SortFunc(members, func(a, b Member) int {
		return cmp.Or(polish.CompareString(a.DisplayName, b.DisplayName), cmp.Compare(a.Id, b.Id))
	})
	return jsonResponse(http.StatusOK, GroupMembers{Members: members})
}

// removeMember takes a user out of the group. A member who names their own ID leaves.
func (s *Server) removeMember(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub, fail := s.member(ctx, req)
	if fail != nil {
		return *fail
	}
	err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		wasMember, err := leaveGroup(ctx, tx, req.PathParameters["userId"], req.PathParameters["groupId"])
		if err == nil && !wasMember {
			return errNotMember
		}
		return err
	})
	if errors.Is(err, errNotMember) {
		return jsonResponse(http.StatusNotFound, Error{Message: "member not found"})
	}
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	return noContent()
}

// leaveGroup takes a user out of a group; wasMember reports that the user was a member. A change of the members
// of a group starts with a write of the group row that changes no value. Two such changes of one group then
// conflict, and db.Tx runs the second one again, so it sees the first.
func leaveGroup(ctx context.Context, tx pgx.Tx, userID, groupID string) (wasMember bool, err error) {
	if _, err := tx.Exec(ctx, "UPDATE groups SET name = name WHERE id = $1", groupID); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, "UPDATE users SET group_id = NULL WHERE id = $1 AND group_id = $2", userID, groupID)
	if err != nil || tag.RowsAffected() == 0 {
		return false, err
	}
	// A group exists only while it has a member.
	_, err = tx.Exec(ctx, "DELETE FROM groups g WHERE g.id = $1 AND NOT EXISTS (SELECT 1 FROM users u WHERE u.group_id = g.id)", groupID)
	return true, err
}
