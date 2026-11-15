package api

import (
	"context"
	"encoding/json/v2"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// pieResult is the result of a done grammar check with the card of grammarBody.
const pieResult = `{"cards":[` + grammarBody + `]}`

func TestCreateGrammarCheck(t *testing.T) {
	const textRule = `{"message":"text must have 1 to 500 characters"}`
	tests := []struct {
		name       string
		sub        string
		body       string
		startErr   error // the error of the fake StartJob
		wantStatus int
		wantBody   string // the exact error body; empty for 202
		wantInput  string // the JSON object of the stored queued check; empty when no check is stored
	}{
		{name: "creates a queued check with the trimmed input", sub: subOla, body: `{"text":"  Idę do sklep.  "}`, wantStatus: 202, wantInput: `{"text":"Idę do sklep."}`},
		{name: "500 letters pass", sub: subOla, body: `{"text":"` + strings.Repeat("ż", 500) + `"}`,
			wantStatus: 202, wantInput: `{"text":"` + strings.Repeat("ż", 500) + `"}`},
		{name: "an unknown field is ignored", sub: subOla, body: `{"text":"Idę do sklep.","meaningRu":"x"}`, wantStatus: 202, wantInput: `{"text":"Idę do sklep."}`},
		{name: "501 letters are rejected", sub: subOla, body: `{"text":"` + strings.Repeat("ż", 501) + `"}`, wantStatus: 400, wantBody: textRule},
		{name: "a body without text is rejected", sub: subOla, body: `{}`, wantStatus: 400, wantBody: textRule},
		{name: "a blank text is rejected", sub: subOla, body: `{"text":"   "}`, wantStatus: 400, wantBody: textRule},
		{name: "a text with a line break is rejected", sub: subOla, body: `{"text":"Idę do sklep.\nMam dwa siostry."}`, wantStatus: 400, wantBody: textRule},
		{name: "a body that is not JSON is rejected", sub: subOla, body: `{"text":`, wantStatus: 400, wantBody: notJSON},
		{name: "a user without AI consent is refused", sub: subAdam, body: `{"text":"Idę do sklep."}`, wantStatus: 403, wantBody: needsConsent},
		{name: "the consent is checked before the body", sub: subAdam, body: `{"text":`, wantStatus: 403, wantBody: needsConsent},
		{name: "a member of another group is refused", sub: subEwa, body: `{"text":"Idę do sklep."}`, wantStatus: 403, wantBody: notMember},
		{name: "a user without a group and without consent is refused as not a member", sub: subJan, body: `{"text":"Idę do sklep."}`, wantStatus: 403, wantBody: notMember},
		{name: "a start that fails answers 502 and keeps the queued check", sub: subOla, body: `{"text":"Idę do sklep."}`, startErr: errors.New("Lambda throttled the call"),
			wantStatus: 502, wantBody: `{"message":"the grammar check did not start"}`, wantInput: `{"text":"Idę do sklep."}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, _ := groupFixture(t)
			if _, err := pool.Exec(ctx, "UPDATE users SET ai_consent_at = now() WHERE id IN ($1, $2)", subOla, subEwa); err != nil {
				t.Fatal(err)
			}
			// stored tells whether the pool, on another connection than the route, saw the job row at the call.
			type start struct {
				userID, jobID string
				stored        bool
			}
			var starts []start
			srv.StartJob = func(ctx context.Context, userID, jobID string) error {
				var stored bool
				if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT FROM jobs WHERE user_id = $1 AND id::text = $2)", userID, jobID).Scan(&stored); err != nil {
					t.Fatal(err)
				}
				starts = append(starts, start{userID: userID, jobID: jobID, stored: stored})
				return tt.startErr
			}

			resp, err := srv.Handle(ctx, groupRequest("POST /v1/groups/{groupId}/grammar-checks", tt.sub, tt.body, map[string]string{"groupId": rodzina}))
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", resp.StatusCode, tt.wantStatus, resp.Body)
			}
			if tt.wantBody != "" && resp.Body != tt.wantBody {
				t.Errorf("body = %s, want %s", resp.Body, tt.wantBody)
			}
			if tt.wantInput == "" {
				if n := countRows(t, pool, "jobs"); n != 0 {
					t.Errorf("jobs = %d, want 0", n)
				}
				if len(starts) != 0 {
					t.Errorf("starts = %+v, want none", starts)
				}
				return
			}
			if len(starts) != 1 {
				t.Fatalf("starts = %+v, want 1", starts)
			}
			started := starts[0]
			if started.userID != tt.sub || !started.stored {
				t.Errorf("start = %+v, want the user %s and a job row that is committed at the call", started, tt.sub)
			}
			if tt.wantBody == "" {
				var got map[string]any
				if err := json.Unmarshal([]byte(resp.Body), &got); err != nil {
					t.Fatalf("body %s: %v", resp.Body, err)
				}
				if len(got) != 2 || got["id"] != started.jobID || got["status"] != "queued" {
					t.Errorf("body = %s, want exactly the id %s of the start and the status queued", resp.Body, started.jobID)
				}
			}
			var n int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE id::text = $1 AND user_id = $2 AND kind = 'grammar' AND input = $3::jsonb AND status = 'queued'",
				started.jobID, tt.sub, tt.wantInput).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 1 {
				t.Errorf("queued checks with the ID %s, the user %s, the kind grammar and the input %s = %d, want 1", started.jobID, tt.sub, tt.wantInput, n)
			}
		})
	}
}

func TestGetGrammarCheck(t *testing.T) {
	const checkNotFound = `{"message":"grammar check not found"}`
	// encoding/json/v2 leaves out a field with omitempty that is an empty list, so the two empty lists are absent.
	// https://pkg.go.dev/encoding/json/v2#hdr-JSON_Representation_of_Go_structs
	card := &[]CardCreate{grammarCreate(func(c *CardCreate) { c.AlternativesPl, c.AlternativesRu = nil, nil })}
	tests := []struct {
		name       string
		setup      string // the SQL text that adds job rows after groupFixture
		sub        string
		checkID    string
		wantStatus int
		wantBody   string       // the exact error body; empty for 200
		want       GrammarCheck // the check of a 200 row; its ID is the ID of the path
	}{
		{name: "a queued check", sub: subOla, checkID: j1, wantStatus: 200, want: GrammarCheck{Status: GrammarCheckStatusQueued},
			setup: addJob + `('sub-ola', '` + j1 + `', 'grammar', 'queued', '{"text":"Idę do sklep."}', NULL, NULL, now())`},
		{name: "a running check", sub: subOla, checkID: j1, wantStatus: 200, want: GrammarCheck{Status: GrammarCheckStatusRunning},
			setup: addJob + `('sub-ola', '` + j1 + `', 'grammar', 'running', '{"text":"Idę do sklep."}', NULL, NULL, now())`},
		{name: "a done check with a card", sub: subOla, checkID: j1, wantStatus: 200, want: GrammarCheck{Status: GrammarCheckStatusDone, Cards: card},
			setup: addJob + `('sub-ola', '` + j1 + `', 'grammar', 'done', '{"text":"Idę do sklep."}', '` + pieResult + `', NULL, now())`},
		{name: "a done check with no card", sub: subOla, checkID: j1, wantStatus: 200, want: GrammarCheck{Status: GrammarCheckStatusDone},
			setup: addJob + `('sub-ola', '` + j1 + `', 'grammar', 'done', '{"text":"Idę do sklep."}', '{"cards":[]}', NULL, now())`},
		{name: "a refused check", sub: subOla, checkID: j1, wantStatus: 200, want: GrammarCheck{Status: GrammarCheckStatusFailed, Error: new(GrammarCheckErrorRefused)},
			setup: addJob + `('sub-ola', '` + j1 + `', 'grammar', 'failed', '{"text":"Idę do sklep."}', NULL, 'refused', now())`},
		{name: "an invalid check", sub: subOla, checkID: j1, wantStatus: 200, want: GrammarCheck{Status: GrammarCheckStatusFailed, Error: new(GrammarCheckErrorInvalid)},
			setup: addJob + `('sub-ola', '` + j1 + `', 'grammar', 'failed', '{"text":"Idę do sklep."}', NULL, 'invalid', now())`},
		{name: "an unavailable check", sub: subOla, checkID: j1, wantStatus: 200, want: GrammarCheck{Status: GrammarCheckStatusFailed, Error: new(GrammarCheckErrorUnavailable)},
			setup: addJob + `('sub-ola', '` + j1 + `', 'grammar', 'failed', '{"text":"Idę do sklep."}', NULL, 'unavailable', now())`},
		{name: "a queued check of 321 seconds reads as failed", sub: subOla, checkID: j1, wantStatus: 200,
			want:  GrammarCheck{Status: GrammarCheckStatusFailed, Error: new(GrammarCheckErrorUnavailable)},
			setup: addJob + `('sub-ola', '` + j1 + `', 'grammar', 'queued', '{"text":"Idę do sklep."}', NULL, NULL, now() - interval '321 seconds')`},
		{name: "a running check of 321 seconds reads as failed", sub: subOla, checkID: j1, wantStatus: 200,
			want:  GrammarCheck{Status: GrammarCheckStatusFailed, Error: new(GrammarCheckErrorUnavailable)},
			setup: addJob + `('sub-ola', '` + j1 + `', 'grammar', 'running', '{"text":"Idę do sklep."}', NULL, NULL, now() - interval '321 seconds')`},
		{name: "a running check of 319 seconds is running", sub: subOla, checkID: j1, wantStatus: 200, want: GrammarCheck{Status: GrammarCheckStatusRunning},
			setup: addJob + `('sub-ola', '` + j1 + `', 'grammar', 'running', '{"text":"Idę do sklep."}', NULL, NULL, now() - interval '319 seconds')`},
		{name: "a done check of 321 seconds stays done", sub: subOla, checkID: j1, wantStatus: 200, want: GrammarCheck{Status: GrammarCheckStatusDone, Cards: card},
			setup: addJob + `('sub-ola', '` + j1 + `', 'grammar', 'done', '{"text":"Idę do sklep."}', '` + pieResult + `', NULL, now() - interval '321 seconds')`},
		{name: "a user without AI consent reads a check", sub: subAdam, checkID: j2, wantStatus: 200, want: GrammarCheck{Status: GrammarCheckStatusQueued},
			setup: addJob + `('sub-adam', '` + j2 + `', 'grammar', 'queued', '{"text":"Idę do sklep."}', NULL, NULL, now())`},
		{name: "the check of another member is not found", sub: subOla, checkID: j2, wantStatus: 404, wantBody: checkNotFound,
			setup: addJob + `('sub-adam', '` + j2 + `', 'grammar', 'queued', '{"text":"Idę do sklep."}', NULL, NULL, now())`},
		{name: "a draft is not found", sub: subOla, checkID: j1, wantStatus: 404, wantBody: checkNotFound,
			setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'queued', '{"text":"Idę do sklep."}', NULL, NULL, now())`},
		{name: "an ID that is not a UUID is not found", sub: subOla, checkID: "nope", wantStatus: 404, wantBody: checkNotFound,
			setup: addJob + `('sub-ola', '` + j1 + `', 'grammar', 'queued', '{"text":"Idę do sklep."}', NULL, NULL, now())`},
		{name: "a member of another group is refused", sub: subEwa, checkID: j1, wantStatus: 403, wantBody: notMember,
			setup: addJob + `('sub-ola', '` + j1 + `', 'grammar', 'queued', '{"text":"Idę do sklep."}', NULL, NULL, now())`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, _ := groupFixture(t)
			if _, err := pool.Exec(ctx, tt.setup); err != nil {
				t.Fatal(err)
			}
			states := func() []string {
				rows, err := pool.Query(ctx, "SELECT concat_ws(' ', id, status, created_at) FROM jobs ORDER BY id")
				if err != nil {
					t.Fatal(err)
				}
				got, err := pgx.CollectRows(rows, pgx.RowTo[string])
				if err != nil {
					t.Fatal(err)
				}
				return got
			}
			before := states()

			resp, err := srv.Handle(ctx, groupRequest("GET /v1/groups/{groupId}/grammar-checks/{checkId}", tt.sub, "", map[string]string{"groupId": rodzina, "checkId": tt.checkID}))
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", resp.StatusCode, tt.wantStatus, resp.Body)
			}
			if after := states(); !slices.Equal(after, before) {
				t.Errorf("jobs = %q, want %q", after, before)
			}
			if tt.wantBody != "" {
				if resp.Body != tt.wantBody {
					t.Errorf("body = %s, want %s", resp.Body, tt.wantBody)
				}
				return
			}
			want := tt.want
			want.Id = tt.checkID
			var got GrammarCheck
			if err := json.Unmarshal([]byte(resp.Body), &got); err != nil {
				t.Fatalf("body %s: %v", resp.Body, err)
			}
			// reflect.DeepEqual treats a nil list and an empty list as different.
			if !reflect.DeepEqual(got, want) {
				t.Errorf("check = %+v, want %+v (body %s)", got, want, resp.Body)
			}
			// A member with the value null decodes as a nil field, so the members are compared by name too.
			var members map[string]any
			if err := json.Unmarshal([]byte(resp.Body), &members); err != nil {
				t.Fatalf("body %s: %v", resp.Body, err)
			}
			var wantKeys []string
			if want.Cards != nil {
				wantKeys = append(wantKeys, "cards")
			}
			if want.Error != nil {
				wantKeys = append(wantKeys, "error")
			}
			wantKeys = append(wantKeys, "id", "status")
			if keys := slices.Sorted(maps.Keys(members)); !slices.Equal(keys, wantKeys) {
				t.Errorf("members = %q, want %q (body %s)", keys, wantKeys, resp.Body)
			}
		})
	}
}
