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

const needsConsent = `{"message":"AI consent is required"}`

// The jobs of the job tests: j1 of Ola and j2 of Adam.
const (
	j1 = "00000000-0000-0000-0000-0000000000a1"
	j2 = "00000000-0000-0000-0000-0000000000a2"
	// addJob takes the rows (user, ID, kind, status, input, result, error, created_at).
	addJob        = "INSERT INTO jobs (user_id, id, kind, status, input, result, error, created_at) VALUES "
	choicesResult = `{"choices":["овощ","оружие"]}`
)

func TestCreateDraft(t *testing.T) {
	const textRule = `{"message":"text must have 1 to 200 characters"}`
	tests := []struct {
		name       string
		sub        string
		body       string
		startErr   error // the error of the fake StartJob
		wantStatus int
		wantBody   string // the exact error body; empty for 202
		wantInput  string // the JSON object of the stored queued job; empty when no job is stored
	}{
		{name: "creates a queued job with the trimmed input", sub: subOla, body: `{"text":"  słowo  "}`, wantStatus: 202, wantInput: `{"text":"słowo"}`},
		{name: "stores the picked meaning", sub: subOla, body: `{"text":"лук","meaningRu":"  овощ  "}`, wantStatus: 202, wantInput: `{"text":"лук","meaningRu":"овощ"}`},
		{name: "a null meaningRu is no meaning", sub: subOla, body: `{"text":"лук","meaningRu":null}`, wantStatus: 202, wantInput: `{"text":"лук"}`},
		{name: "200 letters in text pass", sub: subOla, body: `{"text":"` + strings.Repeat("ż", 200) + `"}`,
			wantStatus: 202, wantInput: `{"text":"` + strings.Repeat("ż", 200) + `"}`},
		{name: "200 letters in meaningRu pass", sub: subOla, body: `{"text":"лук","meaningRu":"` + strings.Repeat("я", 200) + `"}`,
			wantStatus: 202, wantInput: `{"text":"лук","meaningRu":"` + strings.Repeat("я", 200) + `"}`},
		{name: "an unknown field is ignored", sub: subOla, body: `{"text":"dom","kind":"grammar"}`, wantStatus: 202, wantInput: `{"text":"dom"}`},
		{name: "201 letters in text are rejected", sub: subOla, body: `{"text":"` + strings.Repeat("ż", 201) + `"}`, wantStatus: 400, wantBody: textRule},
		{name: "a body without text is rejected", sub: subOla, body: `{}`, wantStatus: 400, wantBody: textRule},
		{name: "a blank text is rejected", sub: subOla, body: `{"text":"   "}`, wantStatus: 400, wantBody: textRule},
		{name: "a text with a line break is rejected", sub: subOla, body: `{"text":"dom\nkot"}`, wantStatus: 400, wantBody: textRule},
		{name: "201 letters in meaningRu are rejected", sub: subOla, body: `{"text":"лук","meaningRu":"` + strings.Repeat("я", 201) + `"}`,
			wantStatus: 400, wantBody: meaningRuRule},
		{name: "an empty meaningRu is rejected", sub: subOla, body: `{"text":"лук","meaningRu":""}`, wantStatus: 400, wantBody: meaningRuRule},
		{name: "the first broken rule decides the text", sub: subOla, body: `{"text":"","meaningRu":""}`, wantStatus: 400, wantBody: textRule},
		{name: "a body that is not JSON is rejected", sub: subOla, body: `{"text":`, wantStatus: 400, wantBody: notJSON},
		{name: "a user without AI consent is refused", sub: subAdam, body: `{"text":"słowo"}`, wantStatus: 403, wantBody: needsConsent},
		{name: "the consent is checked before the body", sub: subAdam, body: `{"text":`, wantStatus: 403, wantBody: needsConsent},
		{name: "a member of another group is refused", sub: subEwa, body: `{"text":"słowo"}`, wantStatus: 403, wantBody: notMember},
		{name: "a user without a group and without consent is refused as not a member", sub: subJan, body: `{"text":"słowo"}`, wantStatus: 403, wantBody: notMember},
		{name: "a start that fails answers 502 and keeps the queued job", sub: subOla, body: `{"text":"słowo"}`, startErr: errors.New("Lambda throttled the call"),
			wantStatus: 502, wantBody: `{"message":"the draft job did not start"}`, wantInput: `{"text":"słowo"}`},
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

			resp, err := srv.Handle(ctx, groupRequest("POST /v1/groups/{groupId}/drafts", tt.sub, tt.body, map[string]string{"groupId": rodzina}))
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
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE id::text = $1 AND user_id = $2 AND kind = 'draft' AND input = $3::jsonb AND status = 'queued'",
				started.jobID, tt.sub, tt.wantInput).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 1 {
				t.Errorf("queued jobs with the ID %s, the user %s, the kind draft and the input %s = %d, want 1", started.jobID, tt.sub, tt.wantInput, n)
			}
		})
	}
}

func TestGetDraft(t *testing.T) {
	const (
		draftNotFound = `{"message":"draft not found"}`
		cardResult    = `{"card":{"pl":"Słowo","ru":"Слово","examplePl":"Nie znam tego słowa.","exampleRu":"Я не знаю этого слова.","meaningRu":"Единица речи","alternativesPl":["Wyraz"],"alternativesRu":[]}}`
	)
	choices := &[]string{"овощ", "оружие"}
	tests := []struct {
		name       string
		setup      string // the SQL text that adds job rows after groupFixture
		sub        string
		draftID    string
		wantStatus int
		wantBody   string // the exact error body; empty for 200
		want       Draft  // the draft of a 200 row; its ID is the ID of the path
	}{
		{name: "a queued job", sub: subOla, draftID: j1, wantStatus: 200, want: Draft{Status: Queued},
			setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'queued', '{"text":"słowo"}', NULL, NULL, now())`},
		{name: "a running job", sub: subOla, draftID: j1, wantStatus: 200, want: Draft{Status: Running},
			setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'running', '{"text":"słowo"}', NULL, NULL, now())`},
		// encoding/json/v2 leaves out a field with omitempty that is an empty list, so alternativesRu is absent.
		// https://pkg.go.dev/encoding/json/v2#hdr-JSON_Representation_of_Go_structs
		{name: "a done job with a card", sub: subOla, draftID: j1, wantStatus: 200,
			setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'done', '{"text":"słowo"}', '` + cardResult + `', NULL, now())`,
			want: Draft{Status: Done, Card: &CardCreate{Pl: "Słowo", Ru: "Слово", ExamplePl: "Nie znam tego słowa.", ExampleRu: "Я не знаю этого слова.",
				MeaningRu: "Единица речи", AlternativesPl: &[]string{"Wyraz"}}}},
		{name: "a done job with meanings", sub: subOla, draftID: j1, wantStatus: 200, want: Draft{Status: Done, Choices: choices},
			setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'done', '{"text":"słowo"}', '` + choicesResult + `', NULL, now())`},
		{name: "a failed job", sub: subOla, draftID: j1, wantStatus: 200, want: Draft{Status: Failed, Error: new(Invalid)},
			setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'failed', '{"text":"słowo"}', NULL, 'invalid', now())`},
		{name: "a queued job of 121 seconds reads as failed", sub: subOla, draftID: j1, wantStatus: 200, want: Draft{Status: Failed, Error: new(Unavailable)},
			setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'queued', '{"text":"słowo"}', NULL, NULL, now() - interval '121 seconds')`},
		{name: "a running job of 121 seconds reads as failed", sub: subOla, draftID: j1, wantStatus: 200, want: Draft{Status: Failed, Error: new(Unavailable)},
			setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'running', '{"text":"słowo"}', NULL, NULL, now() - interval '121 seconds')`},
		{name: "a queued job of 110 seconds is queued", sub: subOla, draftID: j1, wantStatus: 200, want: Draft{Status: Queued},
			setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'queued', '{"text":"słowo"}', NULL, NULL, now() - interval '110 seconds')`},
		{name: "a done job of 121 seconds stays done", sub: subOla, draftID: j1, wantStatus: 200, want: Draft{Status: Done, Choices: choices},
			setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'done', '{"text":"słowo"}', '` + choicesResult + `', NULL, now() - interval '121 seconds')`},
		{name: "a user without AI consent reads a draft", sub: subAdam, draftID: j2, wantStatus: 200, want: Draft{Status: Queued},
			setup: addJob + `('sub-adam', '` + j2 + `', 'draft', 'queued', '{"text":"słowo"}', NULL, NULL, now())`},
		{name: "the job of another member is not found", sub: subOla, draftID: j2, wantStatus: 404, wantBody: draftNotFound,
			setup: addJob + `('sub-adam', '` + j2 + `', 'draft', 'queued', '{"text":"słowo"}', NULL, NULL, now())`},
		{name: "a job of another kind is not found", sub: subOla, draftID: j1, wantStatus: 404, wantBody: draftNotFound,
			setup: addJob + `('sub-ola', '` + j1 + `', 'grammar', 'queued', '{"text":"słowo"}', NULL, NULL, now())`},
		{name: "an ID that is not a UUID is not found", sub: subOla, draftID: "nope", wantStatus: 404, wantBody: draftNotFound,
			setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'queued', '{"text":"słowo"}', NULL, NULL, now())`},
		{name: "a member of another group is refused", sub: subEwa, draftID: j1, wantStatus: 403, wantBody: notMember,
			setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'queued', '{"text":"słowo"}', NULL, NULL, now())`},
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

			resp, err := srv.Handle(ctx, groupRequest("GET /v1/groups/{groupId}/drafts/{draftId}", tt.sub, "", map[string]string{"groupId": rodzina, "draftId": tt.draftID}))
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
			want.Id = tt.draftID
			var got Draft
			if err := json.Unmarshal([]byte(resp.Body), &got); err != nil {
				t.Fatalf("body %s: %v", resp.Body, err)
			}
			// reflect.DeepEqual treats a nil list and an empty list as different.
			if !reflect.DeepEqual(got, want) {
				t.Errorf("draft = %+v, want %+v (body %s)", got, want, resp.Body)
			}
			// A member with the value null decodes as a nil field, so the members are compared by name too.
			var members map[string]any
			if err := json.Unmarshal([]byte(resp.Body), &members); err != nil {
				t.Fatalf("body %s: %v", resp.Body, err)
			}
			var wantKeys []string
			if want.Card != nil {
				wantKeys = append(wantKeys, "card")
			}
			if want.Choices != nil {
				wantKeys = append(wantKeys, "choices")
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

func TestRunJob(t *testing.T) {
	const input = `'{"text":"słowo"}'`
	tests := []struct {
		name  string
		setup string // the SQL text that adds job rows after groupFixture
		event JobEvent
		// The job row of the setup after the call.
		wantStatus string
		wantError  string // empty for NULL
		wantResult string // empty for NULL
	}{
		{name: "claims a queued job and ends it", setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'queued', ` + input + `, NULL, NULL, now())`,
			event: JobEvent{UserID: subOla, JobID: j1}, wantStatus: "failed", wantError: "unavailable"},
		{name: "a job of 50 seconds is claimed", setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'queued', ` + input + `, NULL, NULL, now() - interval '50 seconds')`,
			event: JobEvent{UserID: subOla, JobID: j1}, wantStatus: "failed", wantError: "unavailable"},
		{name: "a job of 61 seconds is not claimed", setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'queued', ` + input + `, NULL, NULL, now() - interval '61 seconds')`,
			event: JobEvent{UserID: subOla, JobID: j1}, wantStatus: "queued"},
		{name: "a running job is not claimed", setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'running', ` + input + `, NULL, NULL, now())`,
			event: JobEvent{UserID: subOla, JobID: j1}, wantStatus: "running"},
		{name: "a done job keeps its result", setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'done', ` + input + `, '` + choicesResult + `', NULL, now())`,
			event: JobEvent{UserID: subOla, JobID: j1}, wantStatus: "done", wantResult: choicesResult},
		{name: "the job of another user is not claimed", setup: addJob + `('sub-adam', '` + j2 + `', 'draft', 'queued', ` + input + `, NULL, NULL, now())`,
			event: JobEvent{UserID: subOla, JobID: j2}, wantStatus: "queued"},
		{name: "an ID that is not a UUID is no error", setup: addJob + `('sub-ola', '` + j1 + `', 'draft', 'queued', ` + input + `, NULL, NULL, now())`,
			event: JobEvent{UserID: subOla, JobID: "nope"}, wantStatus: "queued"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, _, _ := groupFixture(t)
			if _, err := pool.Exec(ctx, tt.setup); err != nil {
				t.Fatal(err)
			}

			if err := srv.RunJob(ctx, tt.event); err != nil {
				t.Errorf("RunJob returned error %v", err)
			}
			var status, jobErr, result string
			var sameResult bool
			if err := pool.QueryRow(ctx, "SELECT status, coalesce(error, ''), coalesce(result::text, ''), result IS NOT DISTINCT FROM nullif($1, '')::jsonb FROM jobs",
				tt.wantResult).Scan(&status, &jobErr, &result, &sameResult); err != nil {
				t.Fatal(err)
			}
			if status != tt.wantStatus {
				t.Errorf("status = %s, want %s", status, tt.wantStatus)
			}
			if jobErr != tt.wantError {
				t.Errorf("error = %q, want %q", jobErr, tt.wantError)
			}
			if !sameResult {
				t.Errorf("result = %q, want %q", result, tt.wantResult)
			}
		})
	}
}

// TestClaimJobAtTheSameInstant makes two claims of one queued job start from the state before the other one.
func TestClaimJobAtTheSameInstant(t *testing.T) {
	ctx := context.Background()
	pool, srv, _, _ := groupFixture(t)
	if _, err := pool.Exec(ctx, addJob+`('sub-ola', '`+j1+`', 'draft', 'queued', '{"text":"słowo"}', NULL, NULL, now())`); err != nil {
		t.Fatal(err)
	}

	// The lock on the job row stops both updates after they have read the queued row. The second update then fails
	// with 40001, and db.Tx runs it again. The rollback at the end frees an update that still waits.
	held, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Rollback(ctx) }()
	if _, err := held.Exec(ctx, "SELECT id FROM jobs WHERE id = $1 FOR UPDATE", j1); err != nil {
		t.Fatal(err)
	}
	claims := make(chan bool, 2)
	for range 2 {
		go func() {
			claimed, err := srv.claimJob(ctx, subOla, j1)
			if err != nil {
				t.Errorf("claimJob returned error %v", err)
			}
			claims <- claimed
		}()
	}
	waitForLockWaits(t, pool, 2)
	if err := held.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	got := []bool{<-claims, <-claims}
	if !slices.Contains(got, true) || !slices.Contains(got, false) {
		t.Errorf("claims = %v, want one true and one false", got)
	}
	var status string
	if err := pool.QueryRow(ctx, "SELECT status FROM jobs WHERE id = $1", j1).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "running" {
		t.Errorf("status = %s, want running", status)
	}
}
