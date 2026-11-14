package claude

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/api"
)

func TestCaller(t *testing.T) {
	// The members of properties are not in alphabetical order. The server must get them in this order.
	const properties = `{"kind":{"type":"string"},"card":{"type":"string"}}`
	req := api.AIRequest{Model: "claude-sonnet-5-5", Effort: "high", MaxTokens: 16000, System: "PROMPT",
		Schema: map[string]any{"type": "object", "properties": jsontext.Value(properties)}, Input: `{"text":"słowo"}`}
	// The body that the server must get for req. It has no thinking, temperature, top_p, top_k, inference_geo,
	// stream or metadata.
	const wantBody = `{"model":"claude-sonnet-5-5","max_tokens":16000,
		"system":[{"type":"text","text":"PROMPT","cache_control":{"type":"ephemeral"}}],
		"output_config":{"effort":"high","format":{"type":"json_schema","schema":{"type":"object","properties":{"kind":{"type":"string"},"card":{"type":"string"}}}}},
		"fallbacks":"default",
		"messages":[{"role":"user","content":[{"type":"text","text":"{\"text\":\"słowo\"}"}]}]}`
	tests := []struct {
		name      string
		hang      bool   // the server does not answer before the context of the call ends
		status    int    // the status of the answer
		requestID string // the header request-id of the answer
		body      string // the body of the answer
		want      api.AIResponse
		wantErr   *api.AIError
	}{
		{
			name: "a thinking block comes before the text", status: http.StatusOK, requestID: "req_1",
			body: `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5-5",
				"content":[{"type":"thinking","thinking":"A card.","signature":"sig"},{"type":"text","text":"{\"a\":1}"}],
				"stop_reason":"end_turn","stop_sequence":null,
				"usage":{"input_tokens":10,"cache_creation_input_tokens":20,"cache_read_input_tokens":30,"output_tokens":40,"inference_geo":"us"}}`,
			want: api.AIResponse{StopReason: "end_turn", Text: `{"a":1}`, Model: "claude-sonnet-5-5", RequestID: "req_1", InferenceGeo: "us",
				Attempts: []api.AIAttempt{{Type: "message", Input: 10, CacheWrite: 20, CacheRead: 30, Output: 40}}},
		},
		{
			// The form of the response is on
			// https://platform.claude.com/docs/en/build-with-claude/refusals-and-fallback#what-the-response-contains
			name: "a fallback served the answer", status: http.StatusOK,
			body: `{"id":"msg_2","type":"message","role":"assistant","model":"claude-sonnet-5",
				"content":[{"type":"fallback","from":{"model":"claude-sonnet-5-5"},"to":{"model":"claude-sonnet-5"},"trigger":{"type":"refusal","category":"cyber"}},
					{"type":"text","text":"{\"b\":2}"}],
				"stop_reason":"end_turn","stop_details":null,
				"usage":{"input_tokens":5,"cache_creation_input_tokens":6,"cache_read_input_tokens":7,"output_tokens":8,"iterations":[
					{"type":"message","model":"claude-sonnet-5-5","input_tokens":1,"cache_creation_input_tokens":2,"cache_read_input_tokens":3,"output_tokens":4},
					{"type":"fallback_message","model":"claude-sonnet-5","input_tokens":5,"cache_creation_input_tokens":6,"cache_read_input_tokens":7,"output_tokens":8}]}}`,
			want: api.AIResponse{StopReason: "end_turn", Text: `{"b":2}`, Model: "claude-sonnet-5", Attempts: []api.AIAttempt{
				{Type: "message", Input: 1, CacheWrite: 2, CacheRead: 3, Output: 4},
				{Type: "fallback_message", Input: 5, CacheWrite: 6, CacheRead: 7, Output: 8}}},
		},
		{
			name: "a refusal", status: http.StatusOK,
			body: `{"id":"msg_3","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[],
				"stop_reason":"refusal","stop_details":{"type":"refusal","category":"cyber"},
				"usage":{"input_tokens":10,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":0}}`,
			want: api.AIResponse{StopReason: "refusal", Model: "claude-sonnet-5-5", Attempts: []api.AIAttempt{{Type: "message", Input: 10}}},
		},
		{
			name: "status 401 is an AIError", status: http.StatusUnauthorized, requestID: "req_2",
			body:    `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"},"request_id":"req_2"}`,
			wantErr: &api.AIError{Status: 401, Type: "authentication_error", RequestID: "req_2"},
		},
		{
			name: "status 529 is an AIError", status: 529,
			body:    `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
			wantErr: &api.AIError{Status: 529, Type: "overloaded_error"},
		},
		{name: "the time limit is not an AIError", hang: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type request struct {
				method, path string
				header       http.Header
				body         []byte
			}
			requests := make(chan request, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				requests <- request{r.Method, r.URL.Path, r.Header, body}
				if tt.hang {
					select {
					case <-r.Context().Done():
					case <-t.Context().Done():
					}
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("request-id", tt.requestID)
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			// t.Context ends before the cleanup, so the handler of the hang row returns before Close waits for it.
			t.Cleanup(srv.Close)

			ctx := t.Context()
			if tt.hang {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}
			client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
			got, err := Caller(client)(ctx, req)

			apiErr, isAPIErr := errors.AsType[*api.AIError](err)
			if tt.hang {
				if err == nil || !errors.Is(err, context.DeadlineExceeded) || isAPIErr {
					t.Fatalf("err = %v, want an error with context.DeadlineExceeded that is not an AIError", err)
				}
				return
			}
			switch {
			case tt.wantErr == nil && err != nil:
				t.Errorf("err = %v, want nil", err)
			// The text of the error is only the AIError, so it has no body of the answer.
			case tt.wantErr != nil && (!isAPIErr || *apiErr != *tt.wantErr || err.Error() != tt.wantErr.Error()):
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("response = %+v, want %+v", got, tt.want)
			}

			var r request
			select {
			case r = <-requests:
			default:
				t.Fatal("the server got no request")
			}
			if r.method != http.MethodPost || r.path != "/v1/messages" {
				t.Errorf("request = %s %s, want POST /v1/messages", r.method, r.path)
			}
			if got := r.header.Get("Authorization"); got != "Bearer test" {
				t.Errorf("Authorization = %q, want Bearer test", got)
			}
			if got := r.header.Get("anthropic-beta"); got != "server-side-fallback-2026-07-01" {
				t.Errorf("anthropic-beta = %q, want server-side-fallback-2026-07-01", got)
			}
			var gotBody, want any
			if err := json.Unmarshal(r.body, &gotBody); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(wantBody), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotBody, want) {
				t.Errorf("body = %s, want %s", r.body, wantBody)
			}
			// The decoded body has no order of members, so the raw body must have the properties text.
			if !bytes.Contains(r.body, []byte(properties)) {
				t.Errorf("body = %s, want the properties %s in this order", r.body, properties)
			}
		})
	}
}
