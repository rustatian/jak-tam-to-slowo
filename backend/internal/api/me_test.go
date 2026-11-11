package api

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
	"github.com/rustatian/jak-tam-to-slowo/backend/internal/dbtest"
	"github.com/rustatian/jak-tam-to-slowo/backend/migrations"
)

func meRequest(sub string) events.APIGatewayV2HTTPRequest {
	return events.APIGatewayV2HTTPRequest{
		RouteKey: "GET /v1/me",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			Authorizer: &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
				JWT: &events.APIGatewayV2HTTPRequestContextAuthorizerJWTDescription{Claims: map[string]string{"sub": sub}},
			},
		},
	}
}

// equalMe compares the consent times by instant, because the database answers in its own time zone.
func equalMe(a, b Me) bool {
	ta, tb := a.AiConsentAt, b.AiConsentAt
	a.AiConsentAt, b.AiConsentAt = nil, nil
	return a == b && (ta == nil) == (tb == nil) && (ta == nil || ta.Equal(*tb))
}

func TestGetMe(t *testing.T) {
	const sub = "6c8f2a4e-1d3b-4f5a-9e7c-2b1a0d9e8f7a"
	created := Me{Id: sub, Email: "learner@example.com", DisplayName: "", Direction: RuPl}
	consentAt := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	existing := Me{Id: sub, Email: "old@example.com", DisplayName: "Ola", Direction: PlRu, Reminder: true, AiConsentAt: &consentAt}
	tests := []struct {
		name        string
		row         *Me
		emailErr    error
		calls       int
		wantStatus  int
		wantMe      Me
		wantBody    string
		wantLookups int32
		wantRows    int
	}{
		{name: "first call creates the row with the Cognito email", calls: 1, wantStatus: 200, wantMe: created, wantLookups: 1, wantRows: 1},
		{name: "second call reads the row without Cognito", row: &existing, calls: 1, wantStatus: 200, wantMe: existing, wantLookups: 0, wantRows: 1},
		{name: "email lookup fails", emailErr: errors.New("cognito down"), calls: 1, wantStatus: 500, wantBody: `{"message":"internal error"}`, wantLookups: 1, wantRows: 0},
		{name: "two first calls make one row", calls: 2, wantStatus: 200, wantMe: created, wantLookups: 2, wantRows: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool := dbtest.New(t)
			if _, err := db.Migrate(ctx, pool, migrations.FS); err != nil {
				t.Fatal(err)
			}
			if tt.row != nil {
				if _, err := pool.Exec(ctx, "INSERT INTO users (id, email, display_name, direction, reminder, ai_consent_at) VALUES ($1, $2, $3, $4, $5, $6)",
					tt.row.Id, tt.row.Email, tt.row.DisplayName, string(tt.row.Direction), tt.row.Reminder, tt.row.AiConsentAt); err != nil {
					t.Fatal(err)
				}
			}
			var lookups atomic.Int32
			var together sync.WaitGroup
			together.Add(tt.calls)
			srv := &Server{DB: pool, EmailOf: func(context.Context, string) (string, error) {
				lookups.Add(1)
				// Every call misses the row before any call inserts it.
				together.Done()
				together.Wait()
				return "learner@example.com", tt.emailErr
			}}

			type result struct {
				resp events.APIGatewayV2HTTPResponse
				err  error
			}
			results := make(chan result, tt.calls)
			for range tt.calls {
				go func() {
					resp, err := srv.Handle(ctx, meRequest(sub))
					results <- result{resp, err}
				}()
			}
			for range tt.calls {
				r := <-results
				if r.err != nil {
					t.Fatalf("Handle returned error %v", r.err)
				}
				if r.resp.StatusCode != tt.wantStatus {
					t.Errorf("status = %d, want %d (body %s)", r.resp.StatusCode, tt.wantStatus, r.resp.Body)
				}
				if got := r.resp.Headers["Content-Type"]; got != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", got)
				}
				if tt.wantBody != "" {
					if r.resp.Body != tt.wantBody {
						t.Errorf("body = %s, want %s", r.resp.Body, tt.wantBody)
					}
					continue
				}
				var me Me
				if err := json.Unmarshal([]byte(r.resp.Body), &me); err != nil {
					t.Fatalf("body %s: %v", r.resp.Body, err)
				}
				if !equalMe(me, tt.wantMe) {
					t.Errorf("me = %+v, want %+v", me, tt.wantMe)
				}
			}
			if got := lookups.Load(); got != tt.wantLookups {
				t.Errorf("Cognito lookups = %d, want %d", got, tt.wantLookups)
			}
			var rows int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE id = $1", sub).Scan(&rows); err != nil {
				t.Fatal(err)
			}
			if rows != tt.wantRows {
				t.Errorf("rows = %d, want %d", rows, tt.wantRows)
			}
		})
	}
}
