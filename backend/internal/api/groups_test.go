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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
	"github.com/rustatian/jak-tam-to-slowo/backend/internal/dbtest"
	"github.com/rustatian/jak-tam-to-slowo/backend/migrations"
)

// The users of groupFixture. subNone has no row.
const (
	subOla  = "sub-ola"
	subAdam = "sub-adam"
	subEwa  = "sub-ewa"
	subJan  = "sub-jan"
	subNone = "sub-none"
)

const (
	notMember = `{"message":"not a member of this group"}`
	nameRule  = `{"message":"name must have 1 to 40 characters"}`
	notJSON   = `{"message":"invalid JSON body"}`
)

// groupFixture makes the group Rodzina with Ola and Adam, the group Praca with Ewa, and Jan without a group.
func groupFixture(t *testing.T) (pool *pgxpool.Pool, srv *Server, rodzina, praca string) {
	t.Helper()
	ctx := context.Background()
	pool = dbtest.New(t)
	if _, err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO groups (name) VALUES ('Rodzina') RETURNING id").Scan(&rodzina); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO groups (name) VALUES ('Praca') RETURNING id").Scan(&praca); err != nil {
		t.Fatal(err)
	}
	user := func(sub, email, name string, group *string) {
		if _, err := pool.Exec(ctx, "INSERT INTO users (id, email, display_name, group_id) VALUES ($1, $2, $3, $4)", sub, email, name, group); err != nil {
			t.Fatal(err)
		}
	}
	user(subOla, "ola@example.com", "Ola", &rodzina)
	user(subAdam, "adam@example.com", "Adam", &rodzina)
	user(subEwa, "ewa@example.com", "Ewa", &praca)
	user(subJan, "jan@example.com", "Jan", nil)
	srv = &Server{DB: pool, EmailOf: func(context.Context, string) (string, error) { return "learner@example.com", nil }}
	return pool, srv, rodzina, praca
}

func groupRequest(routeKey, sub, body string, params map[string]string) events.APIGatewayV2HTTPRequest {
	req := apiRequest(routeKey, sub, body)
	req.PathParameters = params
	return req
}

// groupOf returns the group ID of a user; it is empty without a group and without a row.
func groupOf(t *testing.T, pool *pgxpool.Pool, sub string) string {
	t.Helper()
	var id *string
	if err := pool.QueryRow(context.Background(), "SELECT group_id FROM users WHERE id = $1", sub).Scan(&id); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	if id == nil {
		return ""
	}
	return *id
}

func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// groupRow returns the name and the creation time of a group.
func groupRow(t *testing.T, pool *pgxpool.Pool, id string) (name string, created time.Time) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), "SELECT name, created_at FROM groups WHERE id = $1", id).Scan(&name, &created); err != nil {
		t.Fatalf("group %q: %v", id, err)
	}
	return name, created
}

func TestCreateGroup(t *testing.T) {
	tests := []struct {
		name       string
		sub        string
		body       string
		wantStatus int
		wantBody   string // empty when the answer is the new group
		wantName   string // the name of the new group
		wantEmail  string // the email of the caller's row after the group is made
	}{
		{name: "creates the group with the caller as its member", sub: subJan, body: `{"name":"  Nowa  "}`, wantStatus: 201, wantName: "Nowa", wantEmail: "jan@example.com"},
		{name: "the first call makes the user row", sub: subNone, body: `{"name":"Nowa"}`, wantStatus: 201, wantName: "Nowa", wantEmail: "learner@example.com"},
		{name: "40 letters with diacritics pass", sub: subJan, body: `{"name":"` + strings.Repeat("ż", 40) + `"}`, wantStatus: 201, wantName: strings.Repeat("ż", 40), wantEmail: "jan@example.com"},
		{name: "41 letters are rejected", sub: subJan, body: `{"name":"` + strings.Repeat("ż", 41) + `"}`, wantStatus: 400, wantBody: nameRule},
		{name: "a blank name is rejected", sub: subJan, body: `{"name":"   "}`, wantStatus: 400, wantBody: nameRule},
		{name: "a name with a line break is rejected", sub: subJan, body: `{"name":"Rodzina\nKliknij tu"}`, wantStatus: 400, wantBody: nameRule},
		{name: "a body without a name is rejected", sub: subJan, body: `{}`, wantStatus: 400, wantBody: nameRule},
		{name: "a body that is not JSON is rejected", sub: subJan, body: `{"name":`, wantStatus: 400, wantBody: notJSON},
		{name: "a member of a group gets a conflict", sub: subOla, body: `{"name":"Nowa"}`, wantStatus: 409, wantBody: `{"message":"already in a group"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, _, _ := groupFixture(t)
			before := groupOf(t, pool, tt.sub)

			resp, err := srv.Handle(ctx, apiRequest("POST /v1/groups", tt.sub, tt.body))
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
				if got := countRows(t, pool, "groups"); got != 2 {
					t.Errorf("groups = %d, want the 2 of the fixture", got)
				}
				if got := groupOf(t, pool, tt.sub); got != before {
					t.Errorf("group of the caller = %q, want %q", got, before)
				}
				return
			}
			var group Group
			if err := json.Unmarshal([]byte(resp.Body), &group); err != nil {
				t.Fatalf("body %s: %v", resp.Body, err)
			}
			if got := groupOf(t, pool, tt.sub); got == "" || got != group.Id {
				t.Fatalf("group of the caller = %q, want the new group %q", got, group.Id)
			}
			name, created := groupRow(t, pool, group.Id)
			if group.Name != tt.wantName || name != tt.wantName || !group.CreatedAt.Equal(created) {
				t.Errorf("group = %+v, row = %q %v; want the name %q and the time of the row", group, name, created, tt.wantName)
			}
			if got := countRows(t, pool, "groups"); got != 3 {
				t.Errorf("groups = %d, want 3: the 2 of the fixture and 1 new", got)
			}
			var email string
			if err := pool.QueryRow(ctx, "SELECT email FROM users WHERE id = $1", tt.sub).Scan(&email); err != nil {
				t.Fatal(err)
			}
			if email != tt.wantEmail {
				t.Errorf("email of the caller = %q, want %q", email, tt.wantEmail)
			}
		})
	}
}

func TestCreateGroupTwice(t *testing.T) {
	ctx := context.Background()
	pool, srv, _, _ := groupFixture(t)

	statuses := make(chan int, 2)
	for range 2 {
		go func() {
			resp, err := srv.Handle(ctx, apiRequest("POST /v1/groups", subJan, `{"name":"Nowa"}`))
			if err != nil {
				t.Errorf("Handle returned error %v", err)
			}
			statuses <- resp.StatusCode
		}()
	}
	got := []int{<-statuses, <-statuses}
	slices.Sort(got)
	if !slices.Equal(got, []int{201, 409}) {
		t.Errorf("statuses = %v, want 201 and 409", got)
	}
	if n := countRows(t, pool, "groups"); n != 3 {
		t.Errorf("groups = %d, want 3: the 2 of the fixture and 1 new", n)
	}
	if name, _ := groupRow(t, pool, groupOf(t, pool, subJan)); name != "Nowa" {
		t.Errorf("group of the caller = %q, want Nowa", name)
	}
}

func TestGroup(t *testing.T) {
	const get, patch = "GET /v1/groups/{groupId}", "PATCH /v1/groups/{groupId}"
	tests := []struct {
		name       string
		routeKey   string
		sub        string
		groupID    string // the group ID of the path; empty for the ID of Rodzina
		body       string
		wantStatus int
		wantBody   string // empty when the answer is the group
		wantName   string // the name of Rodzina after the call
	}{
		{name: "a member reads the group", routeKey: get, sub: subOla, wantStatus: 200, wantName: "Rodzina"},
		{name: "a member of another group is refused", routeKey: get, sub: subEwa, wantStatus: 403, wantBody: notMember, wantName: "Rodzina"},
		{name: "a user without a group is refused", routeKey: get, sub: subJan, wantStatus: 403, wantBody: notMember, wantName: "Rodzina"},
		{name: "a user without a row is refused", routeKey: get, sub: subNone, wantStatus: 403, wantBody: notMember, wantName: "Rodzina"},
		{name: "an ID that is not a group is refused", routeKey: get, sub: subOla, groupID: "nope", wantStatus: 403, wantBody: notMember, wantName: "Rodzina"},
		{name: "a member renames the group, trimmed", routeKey: patch, sub: subOla, body: `{"name":"  Dom  "}`, wantStatus: 200, wantName: "Dom"},
		{name: "an empty body keeps the name", routeKey: patch, sub: subOla, body: `{}`, wantStatus: 200, wantName: "Rodzina"},
		{name: "a blank name is rejected", routeKey: patch, sub: subOla, body: `{"name":"   "}`, wantStatus: 400, wantBody: nameRule, wantName: "Rodzina"},
		{name: "a name with a line break is rejected", routeKey: patch, sub: subOla, body: `{"name":"Dom\nKliknij tu"}`, wantStatus: 400, wantBody: nameRule, wantName: "Rodzina"},
		{name: "a body that is not JSON is rejected", routeKey: patch, sub: subOla, body: `{"name":`, wantStatus: 400, wantBody: notJSON, wantName: "Rodzina"},
		{name: "a member of another group cannot rename", routeKey: patch, sub: subEwa, body: `{"name":"Dom"}`, wantStatus: 403, wantBody: notMember, wantName: "Rodzina"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, _ := groupFixture(t)
			groupID := tt.groupID
			if groupID == "" {
				groupID = rodzina
			}

			resp, err := srv.Handle(ctx, groupRequest(tt.routeKey, tt.sub, tt.body, map[string]string{"groupId": groupID}))
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", resp.StatusCode, tt.wantStatus, resp.Body)
			}
			name, created := groupRow(t, pool, rodzina)
			if name != tt.wantName {
				t.Errorf("name of the row = %q, want %q", name, tt.wantName)
			}
			if n := countRows(t, pool, "users"); n != 4 {
				t.Errorf("users = %d, want the 4 of the fixture", n)
			}
			if tt.wantBody != "" {
				if resp.Body != tt.wantBody {
					t.Errorf("body = %s, want %s", resp.Body, tt.wantBody)
				}
				return
			}
			var group Group
			if err := json.Unmarshal([]byte(resp.Body), &group); err != nil {
				t.Fatalf("body %s: %v", resp.Body, err)
			}
			if group.Id != rodzina || group.Name != tt.wantName || !group.CreatedAt.Equal(created) {
				t.Errorf("group = %+v, want the row %s %q %v", group, rodzina, tt.wantName, created)
			}
		})
	}
}

// groupNameOf returns the name of a user's group; it is empty without a group.
func groupNameOf(t *testing.T, pool *pgxpool.Pool, sub string) string {
	t.Helper()
	var name string
	err := pool.QueryRow(context.Background(), "SELECT g.name FROM users u JOIN groups g ON g.id = u.group_id WHERE u.id = $1", sub).Scan(&name)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	return name
}

func TestGroupMembers(t *testing.T) {
	tests := []struct {
		name        string
		setup       string // a statement that changes the fixture before the call
		sub         string
		wantStatus  int
		wantBody    string // empty when the answer is the member list
		wantMembers []Member
	}{
		{name: "lists the members by display name", sub: subOla, wantStatus: 200, wantMembers: []Member{{Id: subAdam, DisplayName: "Adam"}, {Id: subOla, DisplayName: "Ola"}}},
		{name: "a member of another group is refused", sub: subEwa, wantStatus: 403, wantBody: notMember},
		// Polish order: the case does not decide, and ą is a letter after a. Byte order gives Az, Zofia, adam, Ąb.
		{name: "sorts the names for Polish readers", sub: subOla, wantStatus: 200,
			setup: `UPDATE users SET group_id = (SELECT id FROM groups WHERE name = 'Rodzina'),
				display_name = CASE id WHEN 'sub-ola' THEN 'Zofia' WHEN 'sub-adam' THEN 'adam' WHEN 'sub-ewa' THEN 'Ąb' ELSE 'Az' END`,
			wantMembers: []Member{{Id: subAdam, DisplayName: "adam"}, {Id: subJan, DisplayName: "Az"}, {Id: subEwa, DisplayName: "Ąb"}, {Id: subOla, DisplayName: "Zofia"}}},
		{name: "members with one name are in the order of their IDs", sub: subOla, wantStatus: 200,
			setup:       `UPDATE users SET display_name = 'Ola' WHERE id IN ('sub-ola', 'sub-adam')`,
			wantMembers: []Member{{Id: subAdam, DisplayName: "Ola"}, {Id: subOla, DisplayName: "Ola"}}},
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

			resp, err := srv.Handle(ctx, groupRequest("GET /v1/groups/{groupId}/members", tt.sub, "", map[string]string{"groupId": rodzina}))
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
			var list GroupMembers
			if err := json.Unmarshal([]byte(resp.Body), &list); err != nil {
				t.Fatalf("body %s: %v", resp.Body, err)
			}
			if !slices.Equal(list.Members, tt.wantMembers) {
				t.Errorf("members = %+v, want %+v", list.Members, tt.wantMembers)
			}
		})
	}
}

func TestRemoveMember(t *testing.T) {
	const notFound = `{"message":"member not found"}`
	tests := []struct {
		name       string
		sub        string // the caller
		group      string // the name of the group in the path
		userID     string // the user in the path
		wantStatus int
		wantBody   string
		// The state after the call: the name of each user's group, empty for none, and the names of the groups that exist.
		wantOla, wantAdam, wantEwa string
		wantGroups                 []string
	}{
		{name: "a member removes another member", sub: subOla, group: "Rodzina", userID: subAdam, wantStatus: 204,
			wantOla: "Rodzina", wantAdam: "", wantEwa: "Praca", wantGroups: []string{"Praca", "Rodzina"}},
		{name: "a member leaves and the group stays", sub: subOla, group: "Rodzina", userID: subOla, wantStatus: 204,
			wantOla: "", wantAdam: "Rodzina", wantEwa: "Praca", wantGroups: []string{"Praca", "Rodzina"}},
		{name: "the last member leaves and the group is deleted", sub: subEwa, group: "Praca", userID: subEwa, wantStatus: 204,
			wantOla: "Rodzina", wantAdam: "Rodzina", wantEwa: "", wantGroups: []string{"Rodzina"}},
		{name: "a user of another group is not a member", sub: subOla, group: "Rodzina", userID: subEwa, wantStatus: 404, wantBody: notFound,
			wantOla: "Rodzina", wantAdam: "Rodzina", wantEwa: "Praca", wantGroups: []string{"Praca", "Rodzina"}},
		{name: "an unknown user is not a member", sub: subOla, group: "Rodzina", userID: subNone, wantStatus: 404, wantBody: notFound,
			wantOla: "Rodzina", wantAdam: "Rodzina", wantEwa: "Praca", wantGroups: []string{"Praca", "Rodzina"}},
		{name: "a member of another group cannot remove", sub: subEwa, group: "Rodzina", userID: subOla, wantStatus: 403, wantBody: notMember,
			wantOla: "Rodzina", wantAdam: "Rodzina", wantEwa: "Praca", wantGroups: []string{"Praca", "Rodzina"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, praca := groupFixture(t)
			groupID := map[string]string{"Rodzina": rodzina, "Praca": praca}[tt.group]

			resp, err := srv.Handle(ctx, groupRequest("DELETE /v1/groups/{groupId}/members/{userId}", tt.sub, "", map[string]string{"groupId": groupID, "userId": tt.userID}))
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", resp.StatusCode, tt.wantStatus, resp.Body)
			}
			if resp.Body != tt.wantBody {
				t.Errorf("body = %q, want %q", resp.Body, tt.wantBody)
			}
			if got := groupNameOf(t, pool, subOla); got != tt.wantOla {
				t.Errorf("group of Ola = %q, want %q", got, tt.wantOla)
			}
			if got := groupNameOf(t, pool, subAdam); got != tt.wantAdam {
				t.Errorf("group of Adam = %q, want %q", got, tt.wantAdam)
			}
			if got := groupNameOf(t, pool, subEwa); got != tt.wantEwa {
				t.Errorf("group of Ewa = %q, want %q", got, tt.wantEwa)
			}
			if groups := groupNames(t, pool); !slices.Equal(groups, tt.wantGroups) {
				t.Errorf("groups = %v, want %v", groups, tt.wantGroups)
			}
		})
	}
}

func TestLeaveThenCreate(t *testing.T) {
	ctx := context.Background()
	pool, srv, _, praca := groupFixture(t)

	resp, err := srv.Handle(ctx, groupRequest("DELETE /v1/groups/{groupId}/members/{userId}", subEwa, "", map[string]string{"groupId": praca, "userId": subEwa}))
	if err != nil || resp.StatusCode != 204 {
		t.Fatalf("leave: status = %d, error = %v (body %s)", resp.StatusCode, err, resp.Body)
	}
	resp, err = srv.Handle(ctx, apiRequest("POST /v1/groups", subEwa, `{"name":"Nowa"}`))
	if err != nil || resp.StatusCode != 201 {
		t.Fatalf("create: status = %d, error = %v (body %s)", resp.StatusCode, err, resp.Body)
	}
	if got := groupNameOf(t, pool, subEwa); got != "Nowa" {
		t.Errorf("group of the user = %q, want Nowa", got)
	}
}

// groupNames returns the names of the groups that exist, in the order of the names.
func groupNames(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), "SELECT name FROM groups ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return names
}

// memberChange is one request of TestMemberChangesAtTheSameInstant: the caller leaves the group, or accepts the
// pending invitation of the group.
type memberChange struct {
	sub   string
	kind  string // "leave" or "accept"
	group string // the name of the group
}

// waitForLockWaits waits until n sessions of the pool wait for a lock. Each query through the pool is its own
// transaction and reads the current state of the sessions; inside one transaction pg_stat_activity stays the same.
// https://www.postgresql.org/docs/16/monitoring-stats.html#MONITORING-STATS-VIEWS
func waitForLockWaits(t *testing.T, pool *pgxpool.Pool, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int
		err := pool.QueryRow(context.Background(), "SELECT count(*) FROM pg_stat_activity WHERE application_name = current_setting('application_name') AND wait_event_type = 'Lock'").Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting == n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("sessions that wait for a lock = %d, want %d", waiting, n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestMemberChangesAtTheSameInstant sends two requests that change the members of one group, and makes both of
// them start from the state before the other one.
func TestMemberChangesAtTheSameInstant(t *testing.T) {
	tests := []struct {
		name                  string
		first, second         memberChange
		wantFirst, wantSecond int
		// The state after both calls: the name of each user's group, empty for none, and the names of the groups that exist.
		wantOla, wantAdam, wantEwa, wantJan, wantNone string
		wantGroups                                    []string
	}{
		{name: "the two last members leave", first: memberChange{sub: subOla, kind: "leave", group: "Rodzina"}, second: memberChange{sub: subAdam, kind: "leave", group: "Rodzina"},
			wantFirst: 204, wantSecond: 204, wantEwa: "Praca", wantGroups: []string{"Praca"}},
		{name: "the last member leaves, then the invitee accepts", first: memberChange{sub: subEwa, kind: "leave", group: "Praca"}, second: memberChange{sub: subJan, kind: "accept", group: "Praca"},
			wantFirst: 204, wantSecond: 404, wantOla: "Rodzina", wantAdam: "Rodzina", wantGroups: []string{"Rodzina"}},
		{name: "the invitee accepts, then the last member leaves", first: memberChange{sub: subJan, kind: "accept", group: "Praca"}, second: memberChange{sub: subEwa, kind: "leave", group: "Praca"},
			wantFirst: 204, wantSecond: 204, wantOla: "Rodzina", wantAdam: "Rodzina", wantJan: "Praca", wantGroups: []string{"Praca", "Rodzina"}},
		{name: "two users accept one token", first: memberChange{sub: subJan, kind: "accept", group: "Praca"}, second: memberChange{sub: subNone, kind: "accept", group: "Praca"},
			wantFirst: 204, wantSecond: 404, wantOla: "Rodzina", wantAdam: "Rodzina", wantEwa: "Praca", wantJan: "Praca", wantGroups: []string{"Praca", "Rodzina"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, praca := groupFixture(t)
			pendingInvitations(t, pool, rodzina, praca)
			groupID := map[string]string{"Rodzina": rodzina, "Praca": praca}
			token := map[string]string{"Rodzina": tokenRodzina, "Praca": tokenPraca}
			send := func(c memberChange) <-chan int {
				req := groupRequest("DELETE /v1/groups/{groupId}/members/{userId}", c.sub, "", map[string]string{"groupId": groupID[c.group], "userId": c.sub})
				if c.kind == "accept" {
					req = groupRequest("POST /v1/invitations/{token}/acceptance", c.sub, "", map[string]string{"token": token[c.group]})
				}
				status := make(chan int, 1)
				go func() {
					resp, _ := srv.Handle(ctx, req)
					status <- resp.StatusCode
				}()
				return status
			}

			// The locks on the user rows stop each request before its commit: at its write of the user row, or at a row
			// that the other request has written. Each request has then read the state from before the other one.
			// The rollback at the end frees a request that still waits.
			held, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = held.Rollback(ctx) }()
			if _, err := held.Exec(ctx, "SELECT id FROM users WHERE id IN ($1, $2) FOR UPDATE", tt.first.sub, tt.second.sub); err != nil {
				t.Fatal(err)
			}
			first := send(tt.first)
			waitForLockWaits(t, pool, 1)
			second := send(tt.second)
			waitForLockWaits(t, pool, 2)
			if err := held.Commit(ctx); err != nil {
				t.Fatal(err)
			}

			if got := <-first; got != tt.wantFirst {
				t.Errorf("status of the first request = %d, want %d", got, tt.wantFirst)
			}
			if got := <-second; got != tt.wantSecond {
				t.Errorf("status of the second request = %d, want %d", got, tt.wantSecond)
			}
			if got := groupNameOf(t, pool, subOla); got != tt.wantOla {
				t.Errorf("group of Ola = %q, want %q", got, tt.wantOla)
			}
			if got := groupNameOf(t, pool, subAdam); got != tt.wantAdam {
				t.Errorf("group of Adam = %q, want %q", got, tt.wantAdam)
			}
			if got := groupNameOf(t, pool, subEwa); got != tt.wantEwa {
				t.Errorf("group of Ewa = %q, want %q", got, tt.wantEwa)
			}
			if got := groupNameOf(t, pool, subJan); got != tt.wantJan {
				t.Errorf("group of Jan = %q, want %q", got, tt.wantJan)
			}
			if got := groupNameOf(t, pool, subNone); got != tt.wantNone {
				t.Errorf("group of the user without a row = %q, want %q", got, tt.wantNone)
			}
			if groups := groupNames(t, pool); !slices.Equal(groups, tt.wantGroups) {
				t.Errorf("groups = %v, want %v", groups, tt.wantGroups)
			}
		})
	}
}
