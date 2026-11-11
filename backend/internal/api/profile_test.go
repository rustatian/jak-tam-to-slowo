package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
	"github.com/rustatian/jak-tam-to-slowo/backend/internal/dbtest"
	"github.com/rustatian/jak-tam-to-slowo/backend/migrations"
)

func apiRequest(routeKey, sub, body string) events.APIGatewayV2HTTPRequest {
	req := meRequest(sub)
	req.RouteKey = routeKey
	req.Body = body
	return req
}

func TestPatchMe(t *testing.T) {
	const sub = "6c8f2a4e-1d3b-4f5a-9e7c-2b1a0d9e8f7a"
	start := Me{Id: sub, Email: "old@example.com", DisplayName: "", Direction: RuPl}
	with := func(change func(*Me)) Me {
		me := start
		change(&me)
		return me
	}
	const nameRule = `{"message":"displayName must have 1 to 40 characters"}`
	tests := []struct {
		name       string
		noRow      bool
		body       string
		wantStatus int
		wantBody   string
		wantMe     Me
	}{
		{name: "sets the display name, trimmed", body: `{"displayName":"  Ola  "}`, wantStatus: 200, wantMe: with(func(m *Me) { m.DisplayName = "Ola" })},
		{name: "sets direction and reminder", body: `{"direction":"pl-ru","reminder":true}`, wantStatus: 200, wantMe: with(func(m *Me) { m.Direction = PlRu; m.Reminder = true })},
		{name: "an empty body changes nothing", body: `{}`, wantStatus: 200, wantMe: start},
		{name: "40 letters with diacritics pass", body: `{"displayName":"` + strings.Repeat("ż", 40) + `"}`, wantStatus: 200, wantMe: with(func(m *Me) { m.DisplayName = strings.Repeat("ż", 40) })},
		{name: "41 letters are rejected", body: `{"displayName":"` + strings.Repeat("ż", 41) + `"}`, wantStatus: 400, wantBody: nameRule, wantMe: start},
		{name: "a blank display name is rejected", body: `{"displayName":"   "}`, wantStatus: 400, wantBody: nameRule, wantMe: start},
		{name: "an unknown direction is rejected", body: `{"direction":"pl-de"}`, wantStatus: 400, wantBody: `{"message":"direction must be ru-pl or pl-ru"}`, wantMe: start},
		{name: "a body that is not JSON is rejected", body: `{"displayName":`, wantStatus: 400, wantBody: `{"message":"invalid JSON body"}`, wantMe: start},
		{name: "the first call makes the row", noRow: true, body: `{"reminder":true}`, wantStatus: 200, wantMe: with(func(m *Me) { m.Email = "learner@example.com"; m.Reminder = true })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool := dbtest.New(t)
			if _, err := db.Migrate(ctx, pool, migrations.FS); err != nil {
				t.Fatal(err)
			}
			if !tt.noRow {
				if _, err := pool.Exec(ctx, "INSERT INTO users (id, email) VALUES ($1, $2)", start.Id, start.Email); err != nil {
					t.Fatal(err)
				}
			}
			srv := &Server{DB: pool, EmailOf: func(context.Context, string) (string, error) { return "learner@example.com", nil }}

			resp, err := srv.Handle(ctx, apiRequest("PATCH /v1/me", sub, tt.body))
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
			} else {
				var me Me
				if err := json.Unmarshal([]byte(resp.Body), &me); err != nil {
					t.Fatalf("body %s: %v", resp.Body, err)
				}
				if !equalMe(me, tt.wantMe) {
					t.Errorf("response = %+v, want %+v", me, tt.wantMe)
				}
			}
			row, _, err := readUser(ctx, pool, sub)
			if err != nil {
				t.Fatal(err)
			}
			if !equalMe(row, tt.wantMe) {
				t.Errorf("row = %+v, want %+v", row, tt.wantMe)
			}
		})
	}
}
