package api

import (
	"context"
	"crypto/rand"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
)

// ErrEmail reports an address that fails the email rule of invitations.
var ErrEmail = errors.New("email is not valid")

// newGroupMail writes the email of a new group invitation. It has no group and no inviter to name.
func newGroupMail(to, token string) Mail {
	return Mail{
		To:      to,
		Subject: "Zaproszenie do aplikacji „Jak tam to słowo?”",
		Text: "Cześć!\n\nMasz zaproszenie do aplikacji „Jak tam to słowo?”. Po rejestracji założysz w aplikacji własną grupę i zaprosisz do niej inne osoby.\n\n" +
			"Żeby zacząć, otwórz ten link na iPhonie:\n" + inviteLink + token + "\n\n" +
			"Link jest ważny przez 7 dni. Jeśli nie spodziewasz się tego zaproszenia, zignoruj tę wiadomość.\n",
	}
}

// InviteToNewGroup stores a new group invitation for the address and sends its email. The operator command calls
// it. The invitation has no group: its acceptance creates one. The token is only in the email.
func (s *Server) InviteToNewGroup(ctx context.Context, email string) error {
	address, ok := validEmail(email)
	if !ok {
		return ErrEmail
	}
	token := rand.Text()
	var id string
	err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		// One pending new group invitation for an address: a new one replaces it.
		if _, err := tx.Exec(ctx, "DELETE FROM invitations WHERE group_id IS NULL AND email = $1", address); err != nil {
			return err
		}
		return tx.QueryRow(ctx, "INSERT INTO invitations (email, token_hash, expires_at) VALUES ($1, $2, now() + interval '7 days') RETURNING id",
			address, tokenHash(token)).Scan(&id)
	})
	if err != nil {
		return err
	}
	// The send is after the commit: db.Tx can run a transaction again, and the email goes out once.
	m := newGroupMail(address, token)
	m.Invitation = id
	if err := s.Send(ctx, m); err != nil {
		return errors.Join(err, db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "DELETE FROM invitations WHERE id = $1", id)
			return err
		}))
	}
	return nil
}
