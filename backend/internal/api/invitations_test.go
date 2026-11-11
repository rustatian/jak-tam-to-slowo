package api

import (
	"context"
	"encoding/json/v2"
	"errors"
	"strings"
	"testing"
	"time"
)

const emailRule = `{"message":"email is not valid"}`

// sentMail records the emails of a test. When fail is set, every send returns it.
type sentMail struct {
	mails []Mail
	fail  error
}

func (s *sentMail) send(_ context.Context, m Mail) error {
	if s.fail != nil {
		return s.fail
	}
	s.mails = append(s.mails, m)
	return nil
}

func TestValidEmail(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // the address in its stored form; empty when the input is not valid
	}{
		{name: "a plain address passes", input: "ola@example.com", want: "ola@example.com"},
		{name: "spaces and capital letters are removed", input: "  OLA@Example.COM ", want: "ola@example.com"},
		{name: "a plus tag passes", input: "ola+tag@example.com", want: "ola+tag@example.com"},
		{name: "254 characters pass", input: strings.Repeat("a", 242) + "@example.com", want: strings.Repeat("a", 242) + "@example.com"},
		{name: "255 characters are rejected", input: strings.Repeat("a", 243) + "@example.com"},
		{name: "a name with the address is rejected", input: "Ola <ola@example.com>"},
		{name: "angle brackets are rejected", input: "<ola@example.com>"},
		{name: "two addresses are rejected", input: "ola@example.com, adam@example.com"},
		{name: "a domain without a dot is rejected", input: "ola@example"},
		{name: "a missing local part is rejected", input: "@example.com"},
		{name: "letters outside ASCII in the local part are rejected", input: "żółw@example.com"},
		{name: "letters outside ASCII in the domain are rejected", input: "ola@przykład.pl"},
		{name: "an empty text is rejected", input: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := validEmail(tt.input)
			if got != tt.want || ok != (tt.want != "") {
				t.Errorf("validEmail(%q) = %q, %v; want %q, %v", tt.input, got, ok, tt.want, tt.want != "")
			}
		})
	}
}

func TestInvitationMail(t *testing.T) {
	const rest = "\n\nŻeby dołączyć, otwórz ten link na iPhonie:\nhttps://slowo.rustatian.me/invite/ABCDEFGHIJKLMNOPQRSTUVWXYZ\n\n" +
		"Link jest ważny przez 7 dni. Jeśli nie spodziewasz się tego zaproszenia, zignoruj tę wiadomość.\n"
	tests := []struct {
		name     string
		inviter  string
		wantText string
	}{
		{name: "with the name of the inviter", inviter: "Ola", wantText: "Cześć!\n\nOla zaprasza Cię do grupy „Rodzina” w aplikacji „Jak tam to słowo?”." + rest},
		{name: "without a name", inviter: "", wantText: "Cześć!\n\nMasz zaproszenie do grupy „Rodzina” w aplikacji „Jak tam to słowo?”." + rest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := invitationMail("nowy@example.com", tt.inviter, "Rodzina", "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
			want := Mail{To: "nowy@example.com", Subject: "Zaproszenie do grupy w aplikacji „Jak tam to słowo?”", Text: tt.wantText}
			if got != want {
				t.Errorf("mail = %+v\nwant   %+v", got, want)
			}
		})
	}
}

func TestTokenHash(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		// The digest that `printf abc | shasum -a 256` prints.
		{name: "the SHA-256 digest of abc in lower-case hex", token: "abc", want: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tokenHash(tt.token); got != tt.want {
				t.Errorf("tokenHash(%q) = %s, want %s", tt.token, got, tt.want)
			}
		})
	}
}

func TestCreateInvitation(t *testing.T) {
	tests := []struct {
		name        string
		setup       string // a statement that changes the fixture before the call
		sub         string
		body        string
		sendErr     error
		wantStatus  int
		wantBody    string // empty when the answer is the invitation
		wantInviter string // the display name of the inviter in the answer and in the email
		wantPending int    // the pending invitations of nowy@example.com in all groups after the call
	}{
		{name: "stores the invitation and sends one email", sub: subOla, body: `{"email":"  Nowy@Example.com "}`,
			wantStatus: 201, wantInviter: "Ola", wantPending: 1},
		{name: "a second invitation of the address replaces the first", sub: subOla, body: `{"email":"nowy@example.com"}`,
			setup:      `INSERT INTO invitations (group_id, email, token_hash, expires_at) SELECT id, 'nowy@example.com', 'old', now() + interval '1 day' FROM groups WHERE name = 'Rodzina'`,
			wantStatus: 201, wantInviter: "Ola", wantPending: 1},
		{name: "the same address in another group keeps its invitation", sub: subOla, body: `{"email":"nowy@example.com"}`,
			setup:      `INSERT INTO invitations (group_id, email, token_hash, expires_at) SELECT id, 'nowy@example.com', 'praca', now() + interval '1 day' FROM groups WHERE name = 'Praca'`,
			wantStatus: 201, wantInviter: "Ola", wantPending: 2},
		{name: "an inviter without a display name", sub: subOla, body: `{"email":"nowy@example.com"}`,
			setup:      `UPDATE users SET display_name = '' WHERE id = 'sub-ola'`,
			wantStatus: 201, wantInviter: "", wantPending: 1},
		{name: "an address that is not valid is rejected", sub: subOla, body: `{"email":"Ola <ola@example.com>"}`,
			wantStatus: 400, wantBody: emailRule},
		{name: "a body without an email is rejected", sub: subOla, body: `{}`,
			wantStatus: 400, wantBody: emailRule},
		{name: "a body that is not JSON is rejected", sub: subOla, body: `{"email":`,
			wantStatus: 400, wantBody: notJSON},
		{name: "a member of another group is refused", sub: subEwa, body: `{"email":"nowy@example.com"}`,
			wantStatus: 403, wantBody: notMember},
		{name: "a failed send removes the invitation", sub: subOla, body: `{"email":"nowy@example.com"}`, sendErr: errors.New("the email service is down"),
			wantStatus: 502, wantBody: `{"message":"the invitation email was not sent"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, _ := groupFixture(t)
			if tt.setup != "" {
				if _, err := pool.Exec(ctx, tt.setup); err != nil {
					t.Fatal(err)
				}
			}
			sent := &sentMail{fail: tt.sendErr}
			srv.Send = sent.send

			resp, err := srv.Handle(ctx, groupRequest("POST /v1/groups/{groupId}/invitations", tt.sub, tt.body, map[string]string{"groupId": rodzina}))
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", resp.StatusCode, tt.wantStatus, resp.Body)
			}
			var pending int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM invitations WHERE email = 'nowy@example.com' AND accepted_at IS NULL").Scan(&pending); err != nil {
				t.Fatal(err)
			}
			if pending != tt.wantPending {
				t.Errorf("pending invitations of the address = %d, want %d", pending, tt.wantPending)
			}
			if tt.wantBody != "" {
				if resp.Body != tt.wantBody {
					t.Errorf("body = %s, want %s", resp.Body, tt.wantBody)
				}
				if len(sent.mails) != 0 {
					t.Errorf("emails = %d, want none", len(sent.mails))
				}
				return
			}

			var inv Invitation
			if err := json.Unmarshal([]byte(resp.Body), &inv); err != nil {
				t.Fatalf("body %s: %v", resp.Body, err)
			}
			var id, hash string
			var created, expires time.Time
			if err := pool.QueryRow(ctx, "SELECT id, token_hash, created_at, expires_at FROM invitations WHERE group_id = $1 AND email = 'nowy@example.com' AND accepted_at IS NULL", rodzina).
				Scan(&id, &hash, &created, &expires); err != nil {
				t.Fatalf("the row of the invitation: %v", err)
			}
			if inv.Id != id || inv.Email != "nowy@example.com" || !inv.CreatedAt.Equal(created) || !inv.ExpiresAt.Equal(expires) {
				t.Errorf("invitation = %+v, want the row %s nowy@example.com %v %v", inv, id, created, expires)
			}
			if got := inv.ExpiresAt.Sub(inv.CreatedAt); got != 7*24*time.Hour {
				t.Errorf("the invitation is valid for %v, want 7 days", got)
			}
			if inv.InvitedBy == nil || *inv.InvitedBy != (Member{Id: tt.sub, DisplayName: tt.wantInviter}) {
				t.Errorf("invitedBy = %+v, want %s %q", inv.InvitedBy, tt.sub, tt.wantInviter)
			}
			if len(sent.mails) != 1 {
				t.Fatalf("emails = %d, want 1", len(sent.mails))
			}
			_, afterLink, _ := strings.Cut(sent.mails[0].Text, inviteLink)
			token, _, _ := strings.Cut(afterLink, "\n")
			if len(token) < 26 || strings.Trim(token, "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567") != "" {
				t.Errorf("token = %q, want at least 26 base32 characters", token)
			}
			if want := invitationMail("nowy@example.com", tt.wantInviter, "Rodzina", token); sent.mails[0] != want {
				t.Errorf("email = %+v\nwant    %+v", sent.mails[0], want)
			}
			if hash != tokenHash(token) {
				t.Errorf("token_hash = %s, want the hash of the token in the email", hash)
			}
			if strings.Contains(resp.Body, token) {
				t.Errorf("the body has the token: %s", resp.Body)
			}
		})
	}
}
