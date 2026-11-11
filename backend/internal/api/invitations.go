package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
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

// validEmail returns the address without the spaces at its ends and in lower case. ok reports that it is one
// bare address of at most 254 ASCII characters with a dot in its domain. Amazon SES takes only 7-bit ASCII in
// the local part of a destination address.
// https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_Destination.html
func validEmail(s string) (address string, ok bool) {
	address = strings.ToLower(strings.TrimSpace(s))
	if len(address) > 254 || strings.IndexFunc(address, func(r rune) bool { return r > unicode.MaxASCII }) >= 0 {
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
