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
