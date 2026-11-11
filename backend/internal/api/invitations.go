package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"unicode"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
)

// Mail is one plain-text email.
type Mail struct {
	To, Subject, Text string
}

// inviteLink is the start of the universal link of an invitation; the token follows it.
const inviteLink = "https://slowo.rustatian.me/invite/"

// maxInvitesPerDay is the number of invitations that one member can send in one UTC day. Many invitations to
// bad addresses raise the bounce rate of the SES account, which also sends the sign-in codes.
const maxInvitesPerDay = 20

// utcDay is the SQL expression for the current date in UTC.
const utcDay = "(now() AT TIME ZONE 'UTC')::date"

var (
	errTooManyInvitations = errors.New("the member is at the invitation limit of the day")
	errNoInvitation       = errors.New("no pending invitation has the token")
	errAlreadyMember      = errors.New("the user is a member of the group of the invitation")
	errInOtherGroup       = errors.New("the user is in another group")
)

// validEmail returns the address without the spaces at its ends and in lower case. ok reports that it is one
// bare address of at most 254 ASCII characters with a dot in its domain. Amazon SES takes only 7-bit ASCII in
// the local part of a destination address.
// https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_Destination.html
func validEmail(s string) (address string, ok bool) {
	address = strings.ToLower(strings.TrimSpace(s))
	if len(address) > 254 || strings.ContainsFunc(address, func(r rune) bool { return r > unicode.MaxASCII }) {
		return "", false
	}
	// ParseAddress also reads the form "Name <address>". Only a bare address comes back as its own text.
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Address != address {
		return "", false
	}
	if _, domain, _ := strings.Cut(address, "@"); !strings.Contains(domain, ".") {
		return "", false
	}
	return address, true
}

// tokenHash returns the form of an invitation token that the database stores: the hex SHA-256 of the token.
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// invitationMail writes the invitation email. The two names come from users, so they are only in the plain text.
func invitationMail(to, inviter, group, token string) Mail {
	who := inviter + " zaprasza Cię do grupy"
	if inviter == "" {
		who = "Masz zaproszenie do grupy"
	}
	return Mail{
		To:      to,
		Subject: "Zaproszenie do grupy w aplikacji „Jak tam to słowo?”",
		Text: "Cześć!\n\n" + who + " „" + group + "” w aplikacji „Jak tam to słowo?”.\n\n" +
			"Żeby dołączyć, otwórz ten link na iPhonie:\n" + inviteLink + token + "\n\n" +
			"Link jest ważny przez 7 dni. Jeśli nie spodziewasz się tego zaproszenia, zignoruj tę wiadomość.\n",
	}
}

// createInvitation stores an invitation and sends its email. The token is only in the email.
func (s *Server) createInvitation(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub, fail := s.member(ctx, req)
	if fail != nil {
		return *fail
	}
	var body InvitationCreate
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return jsonResponse(http.StatusBadRequest, Error{Message: "invalid JSON body"})
	}
	email, ok := validEmail(body.Email)
	if !ok {
		return jsonResponse(http.StatusBadRequest, Error{Message: "email is not valid"})
	}
	groupID := req.PathParameters["groupId"]
	token := rand.Text()
	invitation := Invitation{Email: email}
	var group, inviter string
	err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		// The limit is in the WHERE clause, so two requests at the same instant cannot both pass at the last one.
		tag, err := tx.Exec(ctx, "UPDATE users SET invites_count = CASE WHEN invites_day = "+utcDay+" THEN invites_count + 1 ELSE 1 END, invites_day = "+utcDay+
			" WHERE id = $1 AND (invites_day IS DISTINCT FROM "+utcDay+" OR invites_count < $2)", sub, maxInvitesPerDay)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errTooManyInvitations
		}
		// One pending invitation for an address in a group: a new one replaces it.
		if _, err := tx.Exec(ctx, "DELETE FROM invitations WHERE group_id = $1 AND email = $2 AND accepted_at IS NULL", groupID, email); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "INSERT INTO invitations (group_id, email, token_hash, invited_by, expires_at) VALUES ($1, $2, $3, $4, now() + interval '7 days') RETURNING id, created_at, expires_at",
			groupID, email, tokenHash(token), sub).Scan(&invitation.Id, &invitation.CreatedAt, &invitation.ExpiresAt); err != nil {
			return err
		}
		return tx.QueryRow(ctx, "SELECT g.name, u.display_name FROM groups g, users u WHERE g.id = $1 AND u.id = $2", groupID, sub).Scan(&group, &inviter)
	})
	if errors.Is(err, errTooManyInvitations) {
		return jsonResponse(http.StatusTooManyRequests, Error{Message: "too many invitations today"})
	}
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	// The send is after the commit: db.Tx can run a transaction again, and the email goes out once.
	if err := s.Send(ctx, invitationMail(email, inviter, group, token)); err != nil {
		slog.ErrorContext(ctx, req.RouteKey, "sub", sub, "error", err)
		err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "DELETE FROM invitations WHERE id = $1", invitation.Id)
			return err
		})
		if err != nil {
			return serverError(ctx, req, sub, err)
		}
		return jsonResponse(http.StatusBadGateway, Error{Message: "the invitation email was not sent"})
	}
	invitation.InvitedBy = &Member{Id: sub, DisplayName: inviter}
	return jsonResponse(http.StatusCreated, invitation)
}

// listInvitations returns the pending invitations of the group.
func (s *Server) listInvitations(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub, fail := s.member(ctx, req)
	if fail != nil {
		return *fail
	}
	rows, err := s.DB.Query(ctx, "SELECT i.id, i.email, i.created_at, i.expires_at, u.id, u.display_name FROM invitations i LEFT JOIN users u ON u.id = i.invited_by "+
		"WHERE i.group_id = $1 AND i.accepted_at IS NULL AND i.expires_at > now() ORDER BY i.created_at, i.id", req.PathParameters["groupId"])
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	invitations, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Invitation, error) {
		var invitation Invitation
		var inviterID, inviterName *string
		if err := row.Scan(&invitation.Id, &invitation.Email, &invitation.CreatedAt, &invitation.ExpiresAt, &inviterID, &inviterName); err != nil {
			return invitation, err
		}
		if inviterID != nil {
			invitation.InvitedBy = &Member{Id: *inviterID, DisplayName: *inviterName}
		}
		return invitation, nil
	})
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	return jsonResponse(http.StatusOK, Invitations{Invitations: invitations})
}

// cancelInvitation deletes a pending invitation of the group.
func (s *Server) cancelInvitation(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub, fail := s.member(ctx, req)
	if fail != nil {
		return *fail
	}
	var cancelled bool
	err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		// The ID is compared as text: PostgreSQL refuses a uuid parameter that is not a UUID (SQLSTATE 22P02).
		// https://www.postgresql.org/docs/16/errcodes-appendix.html
		tag, err := tx.Exec(ctx, "DELETE FROM invitations WHERE id::text = $1 AND group_id = $2 AND accepted_at IS NULL",
			req.PathParameters["invitationId"], req.PathParameters["groupId"])
		cancelled = tag.RowsAffected() == 1
		return err
	})
	if err != nil {
		return serverError(ctx, req, sub, err)
	}
	if !cancelled {
		return jsonResponse(http.StatusNotFound, Error{Message: "invitation not found"})
	}
	return noContent()
}

// previewInvitation shows the group and the inviter of a pending invitation. The route has no authorizer: the token
// of the invitation link is the proof. Every token without a pending invitation gets the same answer.
func (s *Server) previewInvitation(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	var preview InvitationPreview
	// NULLIF gives no name for an inviter whose display name is empty, as for an inviter without an account.
	err := s.DB.QueryRow(ctx, "SELECT g.id, g.name, NULLIF(u.display_name, '') FROM invitations i JOIN groups g ON g.id = i.group_id LEFT JOIN users u ON u.id = i.invited_by "+
		"WHERE i.token_hash = $1 AND i.accepted_at IS NULL AND i.expires_at > now()", tokenHash(req.PathParameters["token"])).
		Scan(&preview.GroupId, &preview.GroupName, &preview.InvitedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return jsonResponse(http.StatusNotFound, Error{Message: "invitation not found"})
	}
	if err != nil {
		return serverError(ctx, req, "", err)
	}
	return jsonResponse(http.StatusOK, preview)
}

// acceptInvitation puts the signed-in user into the group of a pending invitation. The token is the proof, so the
// email of the account can differ from the address of the invitation. A user of another group moves only with
// leaveGroup.
func (s *Server) acceptInvitation(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	sub := subject(req)
	if sub == "" {
		return jsonResponse(http.StatusUnauthorized, Error{Message: "unauthorized"})
	}
	var body InvitationAccept
	if req.Body != "" {
		if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
			return jsonResponse(http.StatusBadRequest, Error{Message: "invalid JSON body"})
		}
	}
	// The first call of a new user can be this one, so the row is made first.
	if _, err := s.me(ctx, sub); err != nil {
		return serverError(ctx, req, sub, err)
	}
	hash := tokenHash(req.PathParameters["token"])
	err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		var invitationID, groupID string
		err := tx.QueryRow(ctx, "SELECT id, group_id FROM invitations WHERE token_hash = $1 AND accepted_at IS NULL AND expires_at > now()", hash).Scan(&invitationID, &groupID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoInvitation
		}
		if err != nil {
			return err
		}
		var current *string
		if err := tx.QueryRow(ctx, "SELECT group_id FROM users WHERE id = $1", sub).Scan(&current); err != nil {
			return err
		}
		if current != nil {
			if *current == groupID {
				return errAlreadyMember
			}
			if body.LeaveGroup == nil || !*body.LeaveGroup {
				return errInOtherGroup
			}
			if _, err := leaveGroup(ctx, tx, sub, *current); err != nil {
				return err
			}
		}
		// A join changes the members of the group too, so it starts with the same write of the group row as leaveGroup.
		if _, err := tx.Exec(ctx, "UPDATE groups SET name = name WHERE id = $1", groupID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE users SET group_id = $1 WHERE id = $2", groupID, sub); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE invitations SET accepted_at = now() WHERE id = $1", invitationID)
		return err
	})
	switch {
	case errors.Is(err, errNoInvitation):
		return jsonResponse(http.StatusNotFound, Error{Message: "invitation not found"})
	case errors.Is(err, errAlreadyMember):
		return jsonResponse(http.StatusConflict, Error{Message: "already a member of this group"})
	case errors.Is(err, errInOtherGroup):
		return jsonResponse(http.StatusConflict, Error{Message: "in another group"})
	case err != nil:
		return serverError(ctx, req, sub, err)
	}
	return noContent()
}
