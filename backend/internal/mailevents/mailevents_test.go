package mailevents

import (
	"context"
	"encoding/json/v2"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
	"github.com/rustatian/jak-tam-to-slowo/backend/internal/dbtest"
	"github.com/rustatian/jak-tam-to-slowo/backend/migrations"
)

// The two files in testdata are a bounce and a complaint as Amazon SES sent them to Amazon EventBridge on
// 2026-11-11 for emails to its mailbox simulator, with the data of the account replaced. Their message tag
// "invitation" has the value invitationID.
const (
	invitationID = "11111111-2222-3333-4444-555555555555"
	otherID      = "22222222-2222-3333-4444-555555555555"
)

func TestHandle(t *testing.T) {
	const tagValue = `"` + invitationID + `"`
	tests := []struct {
		name     string
		events   []string // the files of the events, in the order of their arrival
		old, new string   // a change of the text of each event, when old is set
		wantErr  string   // a part of the text of the error, empty for no error
		want     string   // the failure of the invitation after the events, empty for none
	}{
		{name: "a bounce marks the invitation", events: []string{"bounce.json"}, want: "bounce"},
		{name: "a complaint marks the invitation", events: []string{"complaint.json"}, want: "complaint"},
		{name: "a second delivery of the event gives the same state", events: []string{"bounce.json", "bounce.json"}, want: "bounce"},
		{name: "the last event wins", events: []string{"bounce.json", "complaint.json"}, want: "complaint"},
		{name: "an invitation that does not exist is no error", events: []string{"bounce.json"}, old: tagValue, new: `"99999999-2222-3333-4444-555555555555"`},
		{name: "a tag that is not a UUID is no error", events: []string{"bounce.json"}, old: tagValue, new: `"nope"`},
		{name: "an event without the invitation tag is no error", events: []string{"bounce.json"}, old: `"invitation":`, new: `"other":`},
		{name: "another detail type is an error", events: []string{"bounce.json"}, old: `"Email Bounced"`, new: `"Email Delivered"`, wantErr: "Email Delivered"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool := dbtest.New(t)
			if _, err := db.Migrate(ctx, pool, migrations.FS); err != nil {
				t.Fatal(err)
			}
			var group string
			if err := pool.QueryRow(ctx, "INSERT INTO groups (name) VALUES ('Rodzina') RETURNING id").Scan(&group); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, "INSERT INTO invitations (id, group_id, email, token_hash, expires_at) VALUES ($1, $3, 'a@example.com', 'a', now() + interval '7 days'), ($2, $3, 'b@example.com', 'b', now() + interval '7 days')",
				invitationID, otherID, group); err != nil {
				t.Fatal(err)
			}

			h := &Handler{DB: pool}
			var err error
			for _, file := range tt.events {
				raw, readErr := os.ReadFile("testdata/" + file)
				if readErr != nil {
					t.Fatal(readErr)
				}
				text := string(raw)
				if tt.old != "" {
					if !strings.Contains(text, tt.old) {
						t.Fatalf("%s has no %s", file, tt.old)
					}
					text = strings.ReplaceAll(text, tt.old, tt.new)
				}
				var event events.EventBridgeEvent
				if jsonErr := json.Unmarshal([]byte(text), &event); jsonErr != nil {
					t.Fatal(jsonErr)
				}
				if err = h.Handle(ctx, event); err != nil {
					break
				}
			}
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("Handle returned error %v, want none", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Errorf("Handle returned error %v, want one with %q", err, tt.wantErr)
			}

			var failure, other *string
			if err := pool.QueryRow(ctx, "SELECT (SELECT failure FROM invitations WHERE id = $1), (SELECT failure FROM invitations WHERE id = $2)", invitationID, otherID).Scan(&failure, &other); err != nil {
				t.Fatal(err)
			}
			switch {
			case tt.want == "" && failure != nil:
				t.Errorf("failure of the invitation = %q, want none", *failure)
			case tt.want != "" && (failure == nil || *failure != tt.want):
				t.Errorf("failure of the invitation = %v, want %q", failure, tt.want)
			}
			if other != nil {
				t.Errorf("failure of the other invitation = %q, want none", *other)
			}
		})
	}
}

// testdata/schedule.json is the Input of the schedule in infra/template.yaml. The function gets this text as its event.
func TestHandleSchedule(t *testing.T) {
	tests := []struct {
		name     string
		expires  string // expires_at of the invitation, as an interval from now
		accepted bool   // a person accepted the invitation
		wantKept bool
	}{
		{name: "an expired invitation that nobody accepted is deleted", expires: "-1 second"},
		{name: "an invitation that still works stays", expires: "1 hour", wantKept: true},
		{name: "an expired invitation that a person accepted stays", expires: "-1 second", accepted: true, wantKept: true},
	}
	raw, err := os.ReadFile("testdata/schedule.json")
	if err != nil {
		t.Fatal(err)
	}
	var event events.EventBridgeEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool := dbtest.New(t)
			if _, err := db.Migrate(ctx, pool, migrations.FS); err != nil {
				t.Fatal(err)
			}
			var group string
			if err := pool.QueryRow(ctx, "INSERT INTO groups (name) VALUES ('Rodzina') RETURNING id").Scan(&group); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, "INSERT INTO invitations (group_id, email, token_hash, expires_at, accepted_at) VALUES ($1, 'a@example.com', 'a', now() + $2::interval, CASE WHEN $3 THEN now() END)",
				group, tt.expires, tt.accepted); err != nil {
				t.Fatal(err)
			}

			if err := (&Handler{DB: pool}).Handle(ctx, event); err != nil {
				t.Fatalf("Handle returned error %v, want none", err)
			}

			var kept bool
			if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM invitations)").Scan(&kept); err != nil {
				t.Fatal(err)
			}
			if kept != tt.wantKept {
				t.Errorf("the invitation is kept = %v, want %v", kept, tt.wantKept)
			}
		})
	}
}
