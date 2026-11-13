// Package claude calls the Claude API.
package claude

import (
	"context"
	"errors"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/api"
)

// Caller returns the AI call function for api.Server. The call is the beta Messages call, because the server-side
// fallback to another model is a beta.
// https://platform.claude.com/docs/en/build-with-claude/refusals-and-fallback
func Caller(client anthropic.Client) func(context.Context, api.AIRequest) (api.AIResponse, error) {
	return func(ctx context.Context, req api.AIRequest) (api.AIResponse, error) {
		var httpResp *http.Response
		msg, err := client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
			Model:     req.Model,
			MaxTokens: req.MaxTokens,
			System:    []anthropic.BetaTextBlockParam{{Text: req.System, CacheControl: anthropic.NewBetaCacheControlEphemeralParam()}},
			OutputConfig: anthropic.BetaOutputConfigParam{
				Effort: anthropic.BetaOutputConfigEffort(req.Effort),
				Format: anthropic.BetaJSONOutputFormatParam{Schema: req.Schema},
			},
			Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
			Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
			Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(req.Input))},
		}, option.WithResponseInto(&httpResp))
		// The text of the SDK error has the body of the answer, so only its status, type and request ID go back.
		if apiErr, ok := errors.AsType[*anthropic.Error](err); ok {
			return api.AIResponse{}, &api.AIError{Status: apiErr.StatusCode, Type: string(apiErr.Type()), RequestID: apiErr.RequestID}
		}
		if err != nil {
			return api.AIResponse{}, err
		}

		resp := api.AIResponse{StopReason: string(msg.StopReason), Model: msg.Model, RequestID: httpResp.Header.Get("request-id"), InferenceGeo: msg.Usage.InferenceGeo}
		for _, block := range msg.Content {
			if text, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
				resp.Text += text.Text
			}
		}
		// Each entry of usage.iterations is one attempt. With no entries, the usage of the response is the one attempt.
		// https://platform.claude.com/docs/en/build-with-claude/refusals-and-fallback#what-the-response-contains
		for _, it := range msg.Usage.Iterations {
			resp.Attempts = append(resp.Attempts, api.AIAttempt{Type: it.Type, Input: it.InputTokens, CacheWrite: it.CacheCreationInputTokens, CacheRead: it.CacheReadInputTokens, Output: it.OutputTokens})
		}
		if len(resp.Attempts) == 0 {
			u := msg.Usage
			resp.Attempts = []api.AIAttempt{{Type: "message", Input: u.InputTokens, CacheWrite: u.CacheCreationInputTokens, CacheRead: u.CacheReadInputTokens, Output: u.OutputTokens}}
		}
		return resp, nil
	}
}
