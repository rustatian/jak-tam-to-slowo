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
	"time"

	"github.com/jackc/pgx/v5"
)

// pieResult is the result of a done grammar check with the card of grammarBody.
const pieResult = `{"cards":[` + grammarBody + `]}`

// pieTable is the member table of grammarBody, byte for byte.
const pieTable = `{"title":"Mianownik, liczba mnoga: końcówki według rodzaju","columns":["Rodzaj","Końcówka","Przykład"],"groups":[{"title":"W liczbie mnogiej: męskoosobowy","rows":[{"names":["męski osobowy"],"endings":"-i, -y, -owie, -e","examples":["student → studenci"],"hit":false}]},{"title":"W liczbie mnogiej: niemęskoosobowy","rows":[{"names":["męski żywotny","męski nieżywotny","żeński"],"endings":"-y, -i, -e","examples":["pies → psy","dom → domy","kobieta → kobiety"],"hit":true},{"names":["nijaki"],"endings":"-a","examples":["okno → okna"],"hit":false}]}]}`

// pieCard is the card of grammarBody as the model writes it.
const pieCard = `{"topic":"e ruchome","used":"dwa piesy","right":"dwa psy","sentence":"Na podwórku biegają dwa ___.","answer":"psy","alternatives":[],"hint":"(pies)","facts":["rzeczownik","rodzaj męski żywotny"],"slots":["Mianownik","liczba mnoga","niemęskoosobowy","końcówka -y"],"rule":"W liczbie mnogiej „pies” traci „e”: psy.","examples":["pies → psy","dom → domy","kobieta → kobiety"],"table":` + pieTable + `}`

// grammarAnswer is an answer of the model that ends normally, with the cards.
func grammarAnswer(cards ...string) AIResponse {
	return AIResponse{StopReason: "end_turn", Text: `{"cards":[` + strings.Join(cards, ",") + `]}`}
}

// pieWith is pieCard with each old text of the pairs replaced by the new text.
func pieWith(oldNew ...string) string { return strings.NewReplacer(oldNew...).Replace(pieCard) }

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

func TestRunGrammarCheck(t *testing.T) {
	// reply is the answer and the error of the fake AI for one call.
	type reply struct {
		resp AIResponse
		err  error
	}
	badHint := pieWith(`"hint":"(pies)"`, `"hint":"(dwa piesy, liczba mnoga)"`) // the words of used, and more words
	tests := []struct {
		name    string
		replies []reply // reply i is the answer of call i
		wantErr bool    // RunJob returns an error
		// The job row of the setup after the call.
		wantStatus string
		wantError  string // empty for NULL
		wantResult string // empty for NULL
	}{
		{name: "a card is checked and stored", replies: []reply{{resp: grammarAnswer(pieCard)}}, wantStatus: "done", wantResult: pieResult},
		{name: "the texts of a card are trimmed", replies: []reply{{resp: grammarAnswer(pieWith(`"answer":"psy"`, `"answer":"  psy  "`, `"hint":"(pies)"`, `"hint":" (pies) "`))}},
			wantStatus: "done", wantResult: pieResult},
		{name: "no mistake stores an empty list", replies: []reply{{resp: grammarAnswer()}}, wantStatus: "done", wantResult: `{"cards":[]}`},
		{name: "a null table gives a card without a table", replies: []reply{{resp: grammarAnswer(pieWith(pieTable, "null"))}},
			wantStatus: "done", wantResult: `{"cards":[` + strings.Replace(grammarBody, `,"table":`+pieTable, "", 1) + `]}`},
		{name: "a null group title gives a group without a title",
			replies:    []reply{{resp: grammarAnswer(pieWith(`"title":"W liczbie mnogiej: męskoosobowy",`, `"title":null,`))}},
			wantStatus: "done", wantResult: `{"cards":[` + strings.Replace(grammarBody, `"title":"W liczbie mnogiej: męskoosobowy",`, "", 1) + `]}`},
		{name: "10 cards pass", replies: []reply{{resp: grammarAnswer(slices.Repeat([]string{pieCard}, 10)...)}},
			wantStatus: "done", wantResult: `{"cards":[` + strings.Join(slices.Repeat([]string{grammarBody}, 10), ",") + `]}`},
		{name: "a repeat after an invalid answer", replies: []reply{{resp: grammarAnswer(badHint)}, {resp: grammarAnswer(pieCard)}}, wantStatus: "done", wantResult: pieResult},
		{name: "two invalid answers", replies: []reply{{resp: grammarAnswer(badHint)}, {resp: grammarAnswer(badHint)}}, wantStatus: "failed", wantError: "invalid"},
		{name: "11 cards are invalid two times",
			replies:    []reply{{resp: grammarAnswer(slices.Repeat([]string{pieCard}, 11)...)}, {resp: grammarAnswer(slices.Repeat([]string{pieCard}, 11)...)}},
			wantStatus: "failed", wantError: "invalid"},
		{name: "the output limit is invalid two times",
			replies:    []reply{{resp: AIResponse{StopReason: "max_tokens", Text: grammarAnswer(pieCard).Text}}, {resp: AIResponse{StopReason: "max_tokens", Text: grammarAnswer(pieCard).Text}}},
			wantStatus: "failed", wantError: "invalid"},
		{name: "an unknown member is invalid two times",
			replies:    []reply{{resp: grammarAnswer(pieWith(`"table":`, `"note":"x","table":`))}, {resp: grammarAnswer(pieWith(`"table":`, `"note":"x","table":`))}},
			wantStatus: "failed", wantError: "invalid"},
		{name: "a refusal is not repeated", replies: []reply{{resp: AIResponse{StopReason: "refusal"}}}, wantStatus: "failed", wantError: "refused"},
		{name: "a refusal of the repeat", replies: []reply{{resp: grammarAnswer(badHint)}, {resp: AIResponse{StopReason: "refusal"}}}, wantStatus: "failed", wantError: "refused"},
		{name: "a network error is not repeated", replies: []reply{{err: errors.New("connection reset by peer")}}, wantStatus: "failed", wantError: "unavailable"},
		{name: "the time limit is unavailable", replies: []reply{{err: context.DeadlineExceeded}}, wantStatus: "failed", wantError: "unavailable"},
		{name: "status 401 is unavailable and an error", replies: []reply{{err: &AIError{Status: 401}}}, wantErr: true, wantStatus: "failed", wantError: "unavailable"},
		{name: "status 401 of the repeat is unavailable and an error", replies: []reply{{resp: grammarAnswer(badHint)}, {err: &AIError{Status: 401}}},
			wantErr: true, wantStatus: "failed", wantError: "unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, _, _ := groupFixture(t)
			if _, err := pool.Exec(ctx, addJob+`('sub-ola', '`+j1+`', 'grammar', 'queued', '{"text":"Idę do sklep."}', NULL, NULL, now())`); err != nil {
				t.Fatal(err)
			}
			calls := 0
			srv.AI = func(context.Context, AIRequest) (AIResponse, error) {
				calls++
				if calls > len(tt.replies) {
					t.Errorf("call %d, want at most %d calls", calls, len(tt.replies))
					return AIResponse{}, errors.New("no reply")
				}
				r := tt.replies[calls-1]
				return r.resp, r.err
			}

			if err := srv.RunJob(ctx, JobEvent{UserID: subOla, JobID: j1}); (err != nil) != tt.wantErr {
				t.Errorf("RunJob returned error %v, want an error: %t", err, tt.wantErr)
			}
			if calls != len(tt.replies) {
				t.Errorf("calls = %d, want %d", calls, len(tt.replies))
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

// TestGrammarOutcomeCause checks the cause that the log line of a grammar check gets for an answer of the model.
func TestGrammarOutcomeCause(t *testing.T) {
	badHint := pieWith(`"hint":"(pies)"`, `"hint":"(dwa piesy, liczba mnoga)"`) // the words of used, and more words
	tests := []struct {
		name      string
		resp      AIResponse
		callErr   error
		wantFail  GrammarCheckError
		wantCause string
	}{
		{name: "cards have no cause", resp: grammarAnswer(pieCard), wantFail: "", wantCause: ""},
		{name: "a failed call has no cause", callErr: errors.New("connection reset by peer"), wantFail: GrammarCheckErrorUnavailable, wantCause: ""},
		{name: "a refusal has no cause", resp: AIResponse{StopReason: "refusal"}, wantFail: GrammarCheckErrorRefused, wantCause: ""},
		{name: "the output limit", resp: AIResponse{StopReason: "max_tokens", Text: grammarAnswer(pieCard).Text}, wantFail: GrammarCheckErrorInvalid, wantCause: "stop reason"},
		{name: "a text that is not JSON", resp: AIResponse{StopReason: "end_turn", Text: "psy"}, wantFail: GrammarCheckErrorInvalid, wantCause: "not the JSON of the schema"},
		{name: "an unknown member", resp: grammarAnswer(pieWith(`"table":`, `"note":"x","table":`)), wantFail: GrammarCheckErrorInvalid, wantCause: "not the JSON of the schema"},
		{name: "11 cards", resp: grammarAnswer(slices.Repeat([]string{pieCard}, 11)...), wantFail: GrammarCheckErrorInvalid, wantCause: "11 cards"},
		{name: "a hint with the used form in the second card", resp: grammarAnswer(pieCard, badHint), wantFail: GrammarCheckErrorInvalid,
			wantCause: "card 2: grammar.hint must not contain grammar.used"},
		{name: "a card without hint", resp: grammarAnswer(pieWith(`"hint":"(pies)",`, "")), wantFail: GrammarCheckErrorInvalid,
			wantCause: "card 1: grammar.hint must have 1 to 200 characters"},
		{name: "a sentence without a gap", resp: grammarAnswer(pieWith(`"sentence":"Na podwórku biegają dwa ___."`, `"sentence":"Na podwórku biegają dwa psy."`)),
			wantFail: GrammarCheckErrorInvalid, wantCause: "card 1: examplePl must have ___ exactly once"},
		{name: "a card without facts and slots has no cause",
			resp:     grammarAnswer(pieWith(`"facts":["rzeczownik","rodzaj męski żywotny"],`, "", `"slots":["Mianownik","liczba mnoga","niemęskoosobowy","końcówka -y"],`, "")),
			wantFail: "", wantCause: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, fail, cause := grammarOutcome(tt.resp, tt.callErr)
			if fail != tt.wantFail {
				t.Errorf("fail = %q, want %q", fail, tt.wantFail)
			}
			if cause != tt.wantCause {
				t.Errorf("cause = %q, want %q", cause, tt.wantCause)
			}
		})
	}
}

func TestRunGrammarCheckRequest(t *testing.T) {
	// The schema of the cards keeps the order of its properties on the wire. The top level is a Go map, so
	// Deterministic writes its keys sorted.
	const cardsSchema = `{"type":"array","items":{"type":"object","properties":{"topic":{"type":"string"},"used":{"type":"string"},"right":{"type":"string"},"sentence":{"type":"string"},"answer":{"type":"string"},"alternatives":{"type":"array","items":{"type":"string"}},"hint":{"type":"string"},"facts":{"type":"array","items":{"type":"string"}},"slots":{"type":"array","items":{"type":"string"}},"rule":{"type":"string"},"examples":{"type":"array","items":{"type":"string"},"minItems":1},"table":{"anyOf":[{"type":"object","properties":{"title":{"type":"string"},"columns":{"type":"array","items":{"type":"string"},"minItems":1},"groups":{"type":"array","items":{"type":"object","properties":{"title":{"type":["string","null"]},"rows":{"type":"array","items":{"type":"object","properties":{"names":{"type":"array","items":{"type":"string"},"minItems":1},"endings":{"type":"string"},"examples":{"type":"array","items":{"type":"string"},"minItems":1},"hit":{"type":"boolean"}},"required":["names","endings","examples","hit"],"additionalProperties":false},"minItems":1}},"required":["title","rows"],"additionalProperties":false},"minItems":1}},"required":["title","columns","groups"],"additionalProperties":false},{"type":"null"}]}},"required":["topic","used","right","sentence","answer","alternatives","hint","facts","slots","rule","examples","table"],"additionalProperties":false}}`
	const wantSchema = `{"additionalProperties":false,"properties":{"cards":` + cardsSchema + `},"required":["cards"],"type":"object"}`
	badHint := pieWith(`"hint":"(pies)"`, `"hint":"(dwa piesy, liczba mnoga)"`) // the words of used, and more words
	tests := []struct {
		name  string
		index int // the index of the call
	}{
		{name: "the first call", index: 0},
		{name: "the repeat", index: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, _, _ := groupFixture(t)
			if _, err := pool.Exec(ctx, addJob+`('sub-ola', '`+j1+`', 'grammar', 'queued', '{"text":"Idę do sklep."}', NULL, NULL, now())`); err != nil {
				t.Fatal(err)
			}
			type call struct {
				req         AIRequest
				deadline    time.Time
				hasDeadline bool
			}
			var calls []call
			replies := []AIResponse{grammarAnswer(badHint), grammarAnswer(pieCard)}
			srv.AI = func(ctx context.Context, req AIRequest) (AIResponse, error) {
				deadline, hasDeadline := ctx.Deadline()
				calls = append(calls, call{req: req, deadline: deadline, hasDeadline: hasDeadline})
				if len(calls) > len(replies) {
					return AIResponse{}, errors.New("no reply")
				}
				return replies[len(calls)-1], nil
			}

			before := time.Now()
			if err := srv.RunJob(ctx, JobEvent{UserID: subOla, JobID: j1}); err != nil {
				t.Errorf("RunJob returned error %v", err)
			}
			after := time.Now()
			if len(calls) != 2 {
				t.Fatalf("calls = %d, want 2", len(calls))
			}
			got := calls[tt.index]
			// The schema is compared as text, because the model writes its answer in the order of the schema.
			gotSchema, err := json.Marshal(got.req.Schema, json.Deterministic(true))
			if err != nil {
				t.Fatal(err)
			}
			if string(gotSchema) != wantSchema {
				t.Errorf("schema = %s, want %s", gotSchema, wantSchema)
			}
			got.req.Schema = nil
			if want := (AIRequest{Model: "claude-opus-5-5", Effort: "xhigh", MaxTokens: 16000, System: grammarPrompt, Input: `{"text":"Idę do sklep."}`}); !reflect.DeepEqual(got.req, want) {
				t.Errorf("request = %+v, want %+v", got.req, want)
			}
			if earliest, latest := before.Add(120*time.Second), after.Add(120*time.Second); !got.hasDeadline || got.deadline.Before(earliest) || got.deadline.After(latest) {
				t.Errorf("deadline = %v (set: %t), want from %v to %v", got.deadline, got.hasDeadline, earliest, latest)
			}
		})
	}
}

// TestGrammarPromptExample checks that the worked example of the prompt is an answer that the job stores.
func TestGrammarPromptExample(t *testing.T) {
	_, example, found := strings.Cut(grammarPrompt, `For input {"text":"dwa piesy"}, return `)
	if !found {
		t.Fatal("the prompt has no worked example")
	}
	cards, fail, cause := grammarOutcome(AIResponse{StopReason: "end_turn", Text: strings.TrimSuffix(strings.TrimSpace(example), ".")}, nil)
	if fail != "" || cause != "" || len(cards) != 1 {
		t.Errorf("fail = %q, cause = %q, cards = %d, want no fail, no cause and 1 card", fail, cause, len(cards))
	}
}
