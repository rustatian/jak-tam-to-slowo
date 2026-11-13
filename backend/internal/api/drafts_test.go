package api

import (
	"context"
	"encoding/json/v2"
	"strings"
	"testing"
)

const needsConsent = `{"message":"AI consent is required"}`

func TestCreateDraft(t *testing.T) {
	const textRule = `{"message":"text must have 1 to 200 characters"}`
	tests := []struct {
		name       string
		sub        string
		body       string
		wantStatus int
		wantBody   string // the exact error body; empty for 202
		wantInput  string // the JSON object that a 202 row stores
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, _ := groupFixture(t)
			if _, err := pool.Exec(ctx, "UPDATE users SET ai_consent_at = now() WHERE id IN ($1, $2)", subOla, subEwa); err != nil {
				t.Fatal(err)
			}

			resp, err := srv.Handle(ctx, groupRequest("POST /v1/groups/{groupId}/drafts", tt.sub, tt.body, map[string]string{"groupId": rodzina}))
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
				if n := countRows(t, pool, "jobs"); n != 0 {
					t.Errorf("jobs = %d, want 0", n)
				}
				return
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(resp.Body), &got); err != nil {
				t.Fatalf("body %s: %v", resp.Body, err)
			}
			id, _ := got["id"].(string)
			if len(got) != 2 || id == "" || got["status"] != "queued" {
				t.Fatalf("body = %s, want exactly an id that is not empty and the status queued", resp.Body)
			}
			var n int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE id::text = $1 AND user_id = $2 AND kind = 'draft' AND input = $3::jsonb",
				id, tt.sub, tt.wantInput).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 1 {
				t.Errorf("jobs with the ID %s, the user %s, the kind draft and the input %s = %d, want 1", id, tt.sub, tt.wantInput, n)
			}
		})
	}
}
