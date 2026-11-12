package api

import (
	"context"
	"encoding/json/v2"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5/pgxpool"
)

const emailRule = `{"message":"email is not valid"}`

// sentMail records the emails of a test. When fail is set, every send returns it.
type sentMail struct {
	pool *pgxpool.Pool
	// rowsAtSend has, for each send, the rows with the token of the email as another connection sees them.
	rowsAtSend []int
	mails      []Mail
	fail       error
}

func (s *sentMail) send(ctx context.Context, m Mail) error {
	var rows int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM invitations WHERE token_hash = $1", tokenHash(mailToken(m))).Scan(&rows); err != nil {
		return err
	}
	s.rowsAtSend = append(s.rowsAtSend, rows)
	if s.fail != nil {
		return s.fail
	}
	s.mails = append(s.mails, m)
	return nil
}

// mailToken returns the invitation token in the link of an email.
func mailToken(m Mail) string {
	_, afterLink, _ := strings.Cut(m.Text, inviteLink)
	token, _, _ := strings.Cut(afterLink, "\n")
	return token
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
	const (
		token   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		link    = "https://slowo.rustatian.me/invite/ABCDEFGHIJKLMNOPQRSTUVWXYZ\n\n"
		end     = "Link jest ważny przez 7 dni. Jeśli nie spodziewasz się tego zaproszenia, zignoruj tę wiadomość.\n"
		toGroup = "Zaproszenie do grupy w aplikacji „Jak tam to słowo?”"
		join    = "\n\nŻeby dołączyć, otwórz ten link na iPhonie:\n"
	)
	tests := []struct {
		name        string
		got         Mail
		wantSubject string
		wantText    string
	}{
		{name: "to a group, with the name of the inviter", got: invitationMail("nowy@example.com", "Ola", "Rodzina", token), wantSubject: toGroup,
			wantText: "Cześć!\n\nOla zaprasza Cię do grupy „Rodzina” w aplikacji „Jak tam to słowo?”." + join + link + end},
		{name: "to a group, without a name", got: invitationMail("nowy@example.com", "", "Rodzina", token), wantSubject: toGroup,
			wantText: "Cześć!\n\nMasz zaproszenie do grupy „Rodzina” w aplikacji „Jak tam to słowo?”." + join + link + end},
		{name: "to a new group", got: newGroupMail("nowy@example.com", token), wantSubject: "Zaproszenie do aplikacji „Jak tam to słowo?”",
			wantText: "Cześć!\n\nMasz zaproszenie do aplikacji „Jak tam to słowo?”. Po rejestracji założysz w aplikacji własną grupę i zaprosisz do niej inne osoby." +
				"\n\nŻeby zacząć, otwórz ten link na iPhonie:\n" + link + end},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := Mail{To: "nowy@example.com", Subject: tt.wantSubject, Text: tt.wantText}
			if tt.got != want {
				t.Errorf("mail = %+v\nwant   %+v", tt.got, want)
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
		wantCount   int    // the invitations that the caller sent today (UTC), by the count on the user row
	}{
		{name: "stores the invitation and sends one email", sub: subOla, body: `{"email":"  Nowy@Example.com "}`,
			wantStatus: 201, wantInviter: "Ola", wantPending: 1, wantCount: 1},
		{name: "a second invitation of the address replaces the first", sub: subOla, body: `{"email":"nowy@example.com"}`,
			setup:      `INSERT INTO invitations (group_id, email, token_hash, expires_at) SELECT id, 'nowy@example.com', 'old', now() + interval '1 day' FROM groups WHERE name = 'Rodzina'`,
			wantStatus: 201, wantInviter: "Ola", wantPending: 1, wantCount: 1},
		{name: "the same address in another group keeps its invitation", sub: subOla, body: `{"email":"nowy@example.com"}`,
			setup:      `INSERT INTO invitations (group_id, email, token_hash, expires_at) SELECT id, 'nowy@example.com', 'praca', now() + interval '1 day' FROM groups WHERE name = 'Praca'`,
			wantStatus: 201, wantInviter: "Ola", wantPending: 2, wantCount: 1},
		{name: "an inviter without a display name", sub: subOla, body: `{"email":"nowy@example.com"}`,
			setup:      `UPDATE users SET display_name = '' WHERE id = 'sub-ola'`,
			wantStatus: 201, wantInviter: "", wantPending: 1, wantCount: 1},
		{name: "the 20th invitation of the day passes", sub: subOla, body: `{"email":"nowy@example.com"}`,
			setup:      `UPDATE users SET invites_day = (now() AT TIME ZONE 'UTC')::date, invites_count = 19 WHERE id = 'sub-ola'`,
			wantStatus: 201, wantInviter: "Ola", wantPending: 1, wantCount: 20},
		{name: "the 21st invitation of the day is refused", sub: subOla, body: `{"email":"nowy@example.com"}`,
			setup:      `UPDATE users SET invites_day = (now() AT TIME ZONE 'UTC')::date, invites_count = 20 WHERE id = 'sub-ola'`,
			wantStatus: 429, wantBody: `{"message":"too many invitations today"}`, wantCount: 20},
		{name: "a new day starts the count again", sub: subOla, body: `{"email":"nowy@example.com"}`,
			setup:      `UPDATE users SET invites_day = (now() AT TIME ZONE 'UTC')::date - 1, invites_count = 20 WHERE id = 'sub-ola'`,
			wantStatus: 201, wantInviter: "Ola", wantPending: 1, wantCount: 1},
		{name: "an address that is not valid is rejected", sub: subOla, body: `{"email":"Ola <ola@example.com>"}`,
			wantStatus: 400, wantBody: emailRule},
		{name: "a body without an email is rejected", sub: subOla, body: `{}`,
			wantStatus: 400, wantBody: emailRule},
		{name: "a body that is not JSON is rejected", sub: subOla, body: `{"email":`,
			wantStatus: 400, wantBody: notJSON},
		{name: "a member of another group is refused", sub: subEwa, body: `{"email":"nowy@example.com"}`,
			wantStatus: 403, wantBody: notMember},
		{name: "a failed send removes the invitation and still counts", sub: subOla, body: `{"email":"nowy@example.com"}`, sendErr: errors.New("the email service is down"),
			wantStatus: 502, wantBody: `{"message":"the invitation email was not sent"}`, wantCount: 1},
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
			sent := &sentMail{pool: pool, fail: tt.sendErr}
			srv.Send = sent.send

			resp, err := srv.Handle(ctx, groupRequest("POST /v1/groups/{groupId}/invitations", tt.sub, tt.body, map[string]string{"groupId": rodzina}))
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", resp.StatusCode, tt.wantStatus, resp.Body)
			}
			var pending int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM invitations WHERE email = 'nowy@example.com'").Scan(&pending); err != nil {
				t.Fatal(err)
			}
			if pending != tt.wantPending {
				t.Errorf("pending invitations of the address = %d, want %d", pending, tt.wantPending)
			}
			var count int
			if err := pool.QueryRow(ctx, "SELECT CASE WHEN invites_day = (now() AT TIME ZONE 'UTC')::date THEN invites_count ELSE 0 END FROM users WHERE id = $1", tt.sub).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != tt.wantCount {
				t.Errorf("invitations of the caller today = %d, want %d", count, tt.wantCount)
			}
			// Another connection sees the invitation only after the commit.
			if slices.ContainsFunc(sent.rowsAtSend, func(rows int) bool { return rows != 1 }) {
				t.Errorf("rows of the token at each send = %v, want 1: the send is after the commit", sent.rowsAtSend)
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
			var invitedBy *string
			var created, expires time.Time
			if err := pool.QueryRow(ctx, "SELECT id, token_hash, invited_by, created_at, expires_at FROM invitations WHERE group_id = $1 AND email = 'nowy@example.com'", rodzina).
				Scan(&id, &hash, &invitedBy, &created, &expires); err != nil {
				t.Fatalf("the row of the invitation: %v", err)
			}
			if invitedBy == nil || *invitedBy != tt.sub {
				t.Errorf("invited_by = %v, want %s", invitedBy, tt.sub)
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
			token := mailToken(sent.mails[0])
			if len(token) < 26 || strings.Trim(token, "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567") != "" {
				t.Errorf("token = %q, want at least 26 base32 characters", token)
			}
			want := invitationMail("nowy@example.com", tt.wantInviter, "Rodzina", token)
			want.Invitation = id
			if sent.mails[0] != want {
				t.Errorf("email = %+v\nwant    %+v", sent.mails[0], want)
			}
			if strings.Contains(resp.Body, `"failure"`) {
				t.Errorf("the body of a new invitation has a failure: %s", resp.Body)
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

// invitationRow inserts an invitation and returns its ID. created and expires are offsets from now. An empty group
// makes a new group invitation.
func invitationRow(t *testing.T, pool *pgxpool.Pool, group, email, hash string, invitedBy *string, created, expires time.Duration) string {
	t.Helper()
	now := time.Now()
	var groupID *string
	if group != "" {
		groupID = &group
	}
	var id string
	if err := pool.QueryRow(context.Background(),
		"INSERT INTO invitations (group_id, email, token_hash, invited_by, created_at, expires_at) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id",
		groupID, email, hash, invitedBy, now.Add(created), now.Add(expires)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestListInvitations(t *testing.T) {
	const day = 24 * time.Hour
	// inviter is empty when the answer has no invitedBy, and failure is empty when it has no failure.
	type listed struct{ email, inviter, failure string }
	tests := []struct {
		name       string
		rows       string // the invitation rows of the case: "mixed", "no inviter", "failed" or "none"
		sub        string
		wantStatus int
		wantBody   string // empty when the answer is checked as a list
		want       []listed
	}{
		{name: "lists the pending invitations, oldest first", rows: "mixed", sub: subOla, wantStatus: 200,
			want: []listed{{"a@example.com", "Ola", ""}, {"b@example.com", "Adam", ""}}},
		{name: "an invitation without an inviter has no invitedBy", rows: "no inviter", sub: subOla, wantStatus: 200,
			want: []listed{{"c@example.com", "", ""}}},
		{name: "a failed email shows on the invitation", rows: "failed", sub: subOla, wantStatus: 200,
			want: []listed{{"x@example.com", "", "bounce"}, {"y@example.com", "", "complaint"}, {"z@example.com", "", ""}}},
		{name: "a group without invitations gives an empty list", rows: "none", sub: subOla, wantStatus: 200, wantBody: `{"invitations":[]}`},
		{name: "a member of another group is refused", rows: "mixed", sub: subEwa, wantStatus: 403, wantBody: notMember},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, praca := groupFixture(t)
			ola, adam := subOla, subAdam
			ids := map[string]string{}
			switch tt.rows {
			case "mixed":
				// The newer row goes in first, so the order of the answer comes from created_at.
				ids["b@example.com"] = invitationRow(t, pool, rodzina, "b@example.com", "b", &adam, -1*day, 6*day)
				ids["a@example.com"] = invitationRow(t, pool, rodzina, "a@example.com", "a", &ola, -2*day, 5*day)
				invitationRow(t, pool, rodzina, "expired@example.com", "d", &ola, -8*day, -time.Hour)
				invitationRow(t, pool, praca, "praca@example.com", "e", nil, -time.Hour, 6*day)
				// A new group invitation belongs to no group.
				invitationRow(t, pool, "", "new@example.com", "n", nil, -time.Hour, 6*day)
			case "no inviter":
				ids["c@example.com"] = invitationRow(t, pool, rodzina, "c@example.com", "c", nil, -time.Hour, 6*day)
			case "failed":
				ids["x@example.com"] = invitationRow(t, pool, rodzina, "x@example.com", "x", nil, -3*time.Hour, 6*day)
				ids["y@example.com"] = invitationRow(t, pool, rodzina, "y@example.com", "y", nil, -2*time.Hour, 6*day)
				ids["z@example.com"] = invitationRow(t, pool, rodzina, "z@example.com", "z", nil, -1*time.Hour, 6*day)
				if _, err := pool.Exec(ctx, "UPDATE invitations SET failure = CASE email WHEN 'x@example.com' THEN 'bounce' WHEN 'y@example.com' THEN 'complaint' END"); err != nil {
					t.Fatal(err)
				}
			}

			resp, err := srv.Handle(ctx, groupRequest("GET /v1/groups/{groupId}/invitations", tt.sub, "", map[string]string{"groupId": rodzina}))
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", resp.StatusCode, tt.wantStatus, resp.Body)
			}
			if tt.wantBody != "" {
				if resp.Body != tt.wantBody {
					t.Errorf("body = %s, want %s", resp.Body, tt.wantBody)
				}
				return
			}
			var list Invitations
			if err := json.Unmarshal([]byte(resp.Body), &list); err != nil {
				t.Fatalf("body %s: %v", resp.Body, err)
			}
			if len(list.Invitations) != len(tt.want) {
				t.Fatalf("invitations = %+v, want %d of them", list.Invitations, len(tt.want))
			}
			withInviter := false
			failures := 0
			for i, want := range tt.want {
				got := list.Invitations[i]
				var created, expires time.Time
				if err := pool.QueryRow(ctx, "SELECT created_at, expires_at FROM invitations WHERE id = $1", ids[want.email]).Scan(&created, &expires); err != nil {
					t.Fatal(err)
				}
				if got.Id != ids[want.email] || got.Email != want.email || !got.CreatedAt.Equal(created) || !got.ExpiresAt.Equal(expires) {
					t.Errorf("invitation %d = %+v, want the row %s %s %v %v", i, got, ids[want.email], want.email, created, expires)
				}
				switch {
				case want.inviter == "" && got.InvitedBy != nil:
					t.Errorf("invitation %d: invitedBy = %+v, want none", i, *got.InvitedBy)
				case want.inviter != "" && (got.InvitedBy == nil || got.InvitedBy.DisplayName != want.inviter):
					t.Errorf("invitation %d: invitedBy = %+v, want %q", i, got.InvitedBy, want.inviter)
				}
				withInviter = withInviter || want.inviter != ""
				switch {
				case want.failure == "" && got.Failure != nil:
					t.Errorf("invitation %d: failure = %q, want none", i, *got.Failure)
				case want.failure != "" && (got.Failure == nil || string(*got.Failure) != want.failure):
					t.Errorf("invitation %d: failure = %v, want %q", i, got.Failure, want.failure)
				}
				if want.failure != "" {
					failures++
				}
			}
			// The contract: the member is absent without a failure.
			if got := strings.Count(resp.Body, `"failure"`); got != failures {
				t.Errorf("body %s: failure members = %d, want %d", resp.Body, got, failures)
			}
			// The contract: the member is absent without an inviter.
			if got := strings.Contains(resp.Body, `"invitedBy"`); got != withInviter {
				t.Errorf("body %s: invitedBy present = %v, want %v", resp.Body, got, withInviter)
			}
		})
	}
}

func TestCancelInvitation(t *testing.T) {
	const day = 24 * time.Hour
	const notFound = `{"message":"invitation not found"}`
	tests := []struct {
		name       string
		sub        string
		target     string // the invitation in the path: "pending", "praca", "new group", or the text "nope"
		wantStatus int
		wantBody   string
		wantRows   int // the invitation rows after the call; the case starts with 3
	}{
		{name: "a member cancels a pending invitation", sub: subOla, target: "pending", wantStatus: 204, wantRows: 2},
		{name: "an invitation of another group is not found", sub: subOla, target: "praca", wantStatus: 404, wantBody: notFound, wantRows: 3},
		{name: "a new group invitation is not found", sub: subOla, target: "new group", wantStatus: 404, wantBody: notFound, wantRows: 3},
		{name: "an ID that is not a UUID is not found", sub: subOla, target: "nope", wantStatus: 404, wantBody: notFound, wantRows: 3},
		{name: "a member of another group is refused", sub: subEwa, target: "pending", wantStatus: 403, wantBody: notMember, wantRows: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, praca := groupFixture(t)
			ola := subOla
			ids := map[string]string{
				"pending":   invitationRow(t, pool, rodzina, "a@example.com", "a", &ola, -time.Hour, 6*day),
				"praca":     invitationRow(t, pool, praca, "b@example.com", "b", nil, -time.Hour, 6*day),
				"new group": invitationRow(t, pool, "", "c@example.com", "c", nil, -time.Hour, 6*day),
				"nope":      "nope",
			}

			resp, err := srv.Handle(ctx, groupRequest("DELETE /v1/groups/{groupId}/invitations/{invitationId}", tt.sub, "",
				map[string]string{"groupId": rodzina, "invitationId": ids[tt.target]}))
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", resp.StatusCode, tt.wantStatus, resp.Body)
			}
			if resp.Body != tt.wantBody {
				t.Errorf("body = %q, want %q", resp.Body, tt.wantBody)
			}
			if got := countRows(t, pool, "invitations"); got != tt.wantRows {
				t.Errorf("invitation rows = %d, want %d", got, tt.wantRows)
			}
			if tt.wantStatus == 204 {
				var left int
				if err := pool.QueryRow(ctx, "SELECT count(*) FROM invitations WHERE id = $1", ids[tt.target]).Scan(&left); err != nil {
					t.Fatal(err)
				}
				if left != 0 {
					t.Errorf("the cancelled invitation is still there")
				}
			}
		})
	}
}

// tokenRodzina is the token of the invitation of Rodzina that the tests insert.
const tokenRodzina = "ROD2ZINA3TOKEN4ROD2ZINA3TOK"

const noInvitation = `{"message":"invitation not found"}`

func TestPreviewInvitation(t *testing.T) {
	const day = 24 * time.Hour
	tests := []struct {
		name       string
		row        string // the invitation with tokenRodzina: "pending", "no inviter" or "expired" of Rodzina, or "new group"
		setup      string // a statement that changes the fixture before the call
		token      string
		wantStatus int
		wantBody   string // <rodzina> stands for the ID of the group
	}{
		{name: "a pending invitation shows the group and the inviter", row: "pending", token: tokenRodzina,
			wantStatus: 200, wantBody: `{"groupId":"<rodzina>","groupName":"Rodzina","invitedBy":"Ola"}`},
		{name: "an inviter without a display name is left out", row: "pending", token: tokenRodzina,
			setup:      `UPDATE users SET display_name = '' WHERE id = 'sub-ola'`,
			wantStatus: 200, wantBody: `{"groupId":"<rodzina>","groupName":"Rodzina"}`},
		{name: "an inviter without an account is left out", row: "no inviter", token: tokenRodzina,
			wantStatus: 200, wantBody: `{"groupId":"<rodzina>","groupName":"Rodzina"}`},
		{name: "a new group invitation has no group", row: "new group", token: tokenRodzina, wantStatus: 200, wantBody: `{}`},
		{name: "an unknown token is not found", row: "pending", token: "NOPE", wantStatus: 404, wantBody: noInvitation},
		{name: "an expired invitation is not found", row: "expired", token: tokenRodzina, wantStatus: 404, wantBody: noInvitation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, _ := groupFixture(t)
			ola := subOla
			switch tt.row {
			case "new group":
				invitationRow(t, pool, "", "nowy@example.com", tokenHash(tokenRodzina), nil, -day, 6*day)
			case "pending":
				invitationRow(t, pool, rodzina, "nowy@example.com", tokenHash(tokenRodzina), &ola, -day, 6*day)
			case "no inviter":
				invitationRow(t, pool, rodzina, "nowy@example.com", tokenHash(tokenRodzina), nil, -day, 6*day)
			case "expired":
				invitationRow(t, pool, rodzina, "nowy@example.com", tokenHash(tokenRodzina), &ola, -8*day, -time.Minute)
			}
			if tt.setup != "" {
				if _, err := pool.Exec(ctx, tt.setup); err != nil {
					t.Fatal(err)
				}
			}

			// The request has no authorizer: the route works without sign-in.
			req := events.APIGatewayV2HTTPRequest{RouteKey: "GET /v1/invitations/{token}", PathParameters: map[string]string{"token": tt.token}}
			resp, err := srv.Handle(ctx, req)
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if want := strings.ReplaceAll(tt.wantBody, "<rodzina>", rodzina); resp.Body != want {
				t.Errorf("body = %s, want %s", resp.Body, want)
			}
		})
	}
}

// tokenPraca is the token of the invitation of Praca that the tests insert.
const tokenPraca = "PRA2CA3TOKEN4PRA2CA3TOKEN45"

// tokenNew is the token of the new group invitation that the tests insert.
const tokenNew = "NEW2GROUP3TOKEN4NEW2GROUP34"

// pendingInvitations adds two pending invitations for nowy@example.com: one of Rodzina with tokenRodzina, sent by
// Ola, and one of Praca with tokenPraca, sent by Ewa.
func pendingInvitations(t *testing.T, pool *pgxpool.Pool, rodzina, praca string) {
	t.Helper()
	const day = 24 * time.Hour
	ola, ewa := subOla, subEwa
	invitationRow(t, pool, rodzina, "nowy@example.com", tokenHash(tokenRodzina), &ola, -day, 6*day)
	invitationRow(t, pool, praca, "nowy@example.com", tokenHash(tokenPraca), &ewa, -day, 6*day)
}

func TestAcceptInvitation(t *testing.T) {
	const (
		alreadyMember = `{"message":"already a member of this group"}`
		inOtherGroup  = `{"message":"in another group"}`
		ofRodzina     = "group_id = (SELECT id FROM groups WHERE name = 'Rodzina')"
		groupNameRule = `{"message":"name must have 1 to 40 characters"}`
	)
	both := []string{"Praca", "Rodzina"}
	withNowa := []string{"Nowa", "Praca", "Rodzina"}
	// newGroup adds a new group invitation with tokenNew, so such a case starts with 3 invitation rows.
	newGroup := "INSERT INTO invitations (email, token_hash, expires_at) VALUES ('nowy@example.com', '" + tokenHash(tokenNew) + "', now() + interval '6 days')"
	tests := []struct {
		name       string
		setup      string // a statement that changes the fixture before the call
		sub        string
		token      string
		body       string
		wantStatus int
		wantBody   string
		// The state after the call: the name of the caller's group, empty for none, the names of the groups that
		// exist, whether the invitation of the token is in the table, and the number of invitation rows.
		wantGroup       string
		wantGroups      []string
		wantKept        bool
		wantInvitations int
	}{
		{name: "a user without a group joins, and the address does not need to match", sub: subJan, token: tokenRodzina,
			wantStatus: 204, wantGroup: "Rodzina", wantGroups: both, wantInvitations: 1},
		{name: "a user without a row joins", sub: subNone, token: tokenRodzina,
			wantStatus: 204, wantGroup: "Rodzina", wantGroups: both, wantInvitations: 1},
		{name: "an unknown token is not found", sub: subJan, token: "NOPE",
			wantStatus: 404, wantBody: noInvitation, wantGroups: both, wantInvitations: 2},
		{name: "an expired invitation is not found", sub: subJan, token: tokenRodzina,
			setup:      "UPDATE invitations SET expires_at = now() - interval '1 minute' WHERE " + ofRodzina,
			wantStatus: 404, wantBody: noInvitation, wantGroups: both, wantKept: true, wantInvitations: 2},
		{name: "a member of the group gets a conflict and the invitation stays pending", sub: subAdam, token: tokenRodzina,
			wantStatus: 409, wantBody: alreadyMember, wantGroup: "Rodzina", wantGroups: both, wantKept: true, wantInvitations: 2},
		{name: "a member of another group needs the confirmation", sub: subEwa, token: tokenRodzina,
			wantStatus: 409, wantBody: inOtherGroup, wantGroup: "Praca", wantGroups: both, wantKept: true, wantInvitations: 2},
		{name: "leaveGroup false is not a confirmation", sub: subEwa, token: tokenRodzina, body: `{"leaveGroup":false}`,
			wantStatus: 409, wantBody: inOtherGroup, wantGroup: "Praca", wantGroups: both, wantKept: true, wantInvitations: 2},
		{name: "with the confirmation the last member moves and the old group is deleted", sub: subEwa, token: tokenRodzina, body: `{"leaveGroup":true}`,
			wantStatus: 204, wantGroup: "Rodzina", wantGroups: []string{"Rodzina"}, wantInvitations: 0},
		{name: "with the confirmation a member moves and the old group stays", sub: subAdam, token: tokenPraca, body: `{"leaveGroup":true}`,
			wantStatus: 204, wantGroup: "Praca", wantGroups: both, wantInvitations: 1},
		{name: "a body that is not JSON is rejected", sub: subJan, token: tokenRodzina, body: `{"leaveGroup":`,
			wantStatus: 400, wantBody: notJSON, wantGroups: both, wantKept: true, wantInvitations: 2},
		{name: "a new group invitation creates the group", setup: newGroup, sub: subJan, token: tokenNew, body: `{"groupName":"  Nowa  "}`,
			wantStatus: 204, wantGroup: "Nowa", wantGroups: withNowa, wantInvitations: 2},
		{name: "a new group invitation needs a group name", setup: newGroup, sub: subJan, token: tokenNew,
			wantStatus: 400, wantBody: groupNameRule, wantGroups: both, wantKept: true, wantInvitations: 3},
		{name: "a new group name with a line break is rejected", setup: newGroup, sub: subJan, token: tokenNew, body: `{"groupName":"No\nwa"}`,
			wantStatus: 400, wantBody: groupNameRule, wantGroups: both, wantKept: true, wantInvitations: 3},
		{name: "a member of a group needs the confirmation for a new group", setup: newGroup, sub: subEwa, token: tokenNew, body: `{"groupName":"Nowa"}`,
			wantStatus: 409, wantBody: inOtherGroup, wantGroup: "Praca", wantGroups: both, wantKept: true, wantInvitations: 3},
		{name: "with the confirmation a member leaves and starts the new group", setup: newGroup, sub: subAdam, token: tokenNew, body: `{"groupName":"Nowa","leaveGroup":true}`,
			wantStatus: 204, wantGroup: "Nowa", wantGroups: withNowa, wantInvitations: 2},
		{name: "with the confirmation the last member starts the new group and the old group is deleted", setup: newGroup, sub: subEwa, token: tokenNew, body: `{"groupName":"Nowa","leaveGroup":true}`,
			wantStatus: 204, wantGroup: "Nowa", wantGroups: []string{"Nowa", "Rodzina"}, wantInvitations: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, praca := groupFixture(t)
			pendingInvitations(t, pool, rodzina, praca)
			if tt.setup != "" {
				if _, err := pool.Exec(ctx, tt.setup); err != nil {
					t.Fatal(err)
				}
			}

			resp, err := srv.Handle(ctx, groupRequest("POST /v1/invitations/{token}/acceptance", tt.sub, tt.body, map[string]string{"token": tt.token}))
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if resp.Body != tt.wantBody {
				t.Errorf("body = %q, want %q", resp.Body, tt.wantBody)
			}
			if got := groupNameOf(t, pool, tt.sub); got != tt.wantGroup {
				t.Errorf("group of the caller = %q, want %q", got, tt.wantGroup)
			}
			if groups := groupNames(t, pool); !slices.Equal(groups, tt.wantGroups) {
				t.Errorf("groups = %v, want %v", groups, tt.wantGroups)
			}
			var kept bool
			if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM invitations WHERE token_hash = $1)", tokenHash(tt.token)).Scan(&kept); err != nil {
				t.Fatal(err)
			}
			if kept != tt.wantKept {
				t.Errorf("the invitation of the token is in the table = %t, want %t", kept, tt.wantKept)
			}
			if got := countRows(t, pool, "invitations"); got != tt.wantInvitations {
				t.Errorf("invitation rows = %d, want %d", got, tt.wantInvitations)
			}
		})
	}
}

func TestInviteToNewGroup(t *testing.T) {
	sendFailed := errors.New("the email service is down")
	tests := []struct {
		name    string
		setup   string // a statement that changes the fixture before the call
		email   string
		sendErr error
		wantErr error // nil when the call must pass
		// The invitations of nowy@example.com after the call: those without a group, and those to a group.
		wantNew   int
		wantGroup int
	}{
		{name: "stores a new group invitation and sends one email", email: "  Nowy@Example.com ", wantNew: 1},
		{name: "a second invitation of the address replaces the first", email: "nowy@example.com",
			setup:   `INSERT INTO invitations (email, token_hash, expires_at) VALUES ('nowy@example.com', 'old', now() + interval '1 day')`,
			wantNew: 1},
		{name: "an invitation of the address to a group stays", email: "nowy@example.com",
			setup:   `INSERT INTO invitations (group_id, email, token_hash, expires_at) SELECT id, 'nowy@example.com', 'rodzina', now() + interval '1 day' FROM groups WHERE name = 'Rodzina'`,
			wantNew: 1, wantGroup: 1},
		{name: "an address that is not valid is rejected", email: "Ola <ola@example.com>", wantErr: ErrEmail},
		{name: "a failed send removes the invitation", email: "nowy@example.com", sendErr: sendFailed, wantErr: sendFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, _, _ := groupFixture(t)
			if tt.setup != "" {
				if _, err := pool.Exec(ctx, tt.setup); err != nil {
					t.Fatal(err)
				}
			}
			sent := &sentMail{pool: pool, fail: tt.sendErr}
			srv.Send = sent.send

			if err := srv.InviteToNewGroup(ctx, tt.email); !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
			var withoutGroup, toGroup int
			if err := pool.QueryRow(ctx, "SELECT count(*) FILTER (WHERE group_id IS NULL), count(*) FILTER (WHERE group_id IS NOT NULL) FROM invitations WHERE email = 'nowy@example.com'").
				Scan(&withoutGroup, &toGroup); err != nil {
				t.Fatal(err)
			}
			if withoutGroup != tt.wantNew || toGroup != tt.wantGroup {
				t.Errorf("invitations of the address: %d without a group and %d to a group, want %d and %d", withoutGroup, toGroup, tt.wantNew, tt.wantGroup)
			}
			// Another connection sees the invitation only after the commit.
			if slices.ContainsFunc(sent.rowsAtSend, func(rows int) bool { return rows != 1 }) {
				t.Errorf("rows of the token at each send = %v, want 1: the send is after the commit", sent.rowsAtSend)
			}
			if tt.wantErr != nil {
				if len(sent.mails) != 0 {
					t.Errorf("emails = %d, want none", len(sent.mails))
				}
				return
			}

			if len(sent.mails) != 1 {
				t.Fatalf("emails = %d, want 1", len(sent.mails))
			}
			token := mailToken(sent.mails[0])
			var id, hash string
			var invitedBy *string
			var sevenDays bool
			if err := pool.QueryRow(ctx, "SELECT id, token_hash, invited_by, expires_at - created_at = interval '7 days' FROM invitations WHERE group_id IS NULL AND email = 'nowy@example.com'").
				Scan(&id, &hash, &invitedBy, &sevenDays); err != nil {
				t.Fatalf("the row of the invitation: %v", err)
			}
			if invitedBy != nil {
				t.Errorf("invited_by = %s, want none", *invitedBy)
			}
			if !sevenDays {
				t.Errorf("the invitation is not valid for 7 days")
			}
			if len(token) < 26 || hash != tokenHash(token) {
				t.Errorf("token_hash = %s, want the hash of the token in the email, and a token of at least 26 characters", hash)
			}
			want := newGroupMail("nowy@example.com", token)
			want.Invitation = id
			if sent.mails[0] != want {
				t.Errorf("email = %+v\nwant    %+v", sent.mails[0], want)
			}
		})
	}
}
