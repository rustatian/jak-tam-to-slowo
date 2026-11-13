package api

import "fmt"

// AIRequest is one call of the AI model. Schema is the JSON schema of the answer, and Input is the text of the one
// user message.
type AIRequest struct {
	Model, Effort string
	MaxTokens     int64
	System        string
	Schema        map[string]any
	Input         string
}

// AIAttempt is the token counts of one attempt of a call. Type is the type of the entry of usage.iterations, or
// message when the response has no entries.
// https://platform.claude.com/docs/en/build-with-claude/refusals-and-fallback#what-the-response-contains
type AIAttempt struct {
	Type                                 string
	Input, CacheWrite, CacheRead, Output int64
}

// AIResponse is the answer of the AI model. Text is each text block of the content together.
type AIResponse struct {
	StopReason, Text, Model, RequestID, InferenceGeo string
	Attempts                                         []AIAttempt
}

// AIError is an answer of the AI API with a status outside 2xx. Each other failure of a call, such as the network or
// the time limit, is another error.
type AIError struct {
	Status          int
	Type, RequestID string
}

func (e *AIError) Error() string {
	return fmt.Sprintf("AI API status %d, type %s, request ID %s", e.Status, e.Type, e.RequestID)
}
