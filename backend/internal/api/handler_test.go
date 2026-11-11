package api

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestHandle(t *testing.T) {
	const sub = "6c8f2a4e-1d3b-4f5a-9e7c-2b1a0d9e8f7a"
	jwt := func(claims map[string]string) *events.APIGatewayV2HTTPRequestContextAuthorizerDescription {
		return &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
			JWT: &events.APIGatewayV2HTTPRequestContextAuthorizerJWTDescription{Claims: claims},
		}
	}
	tests := []struct {
		name       string
		routeKey   string
		authorizer *events.APIGatewayV2HTTPRequestContextAuthorizerDescription
		wantStatus int
		wantBody   string
	}{
		{name: "me without an authorizer", routeKey: "GET /v1/me", authorizer: nil, wantStatus: 401, wantBody: `{"message":"unauthorized"}`},
		{name: "me without a JWT authorizer", routeKey: "GET /v1/me", authorizer: &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{}, wantStatus: 401, wantBody: `{"message":"unauthorized"}`},
		{name: "me with an empty sub claim", routeKey: "GET /v1/me", authorizer: jwt(map[string]string{"sub": ""}), wantStatus: 401, wantBody: `{"message":"unauthorized"}`},
		{name: "patch me without an authorizer", routeKey: "PATCH /v1/me", authorizer: nil, wantStatus: 401, wantBody: `{"message":"unauthorized"}`},
		{name: "give consent without an authorizer", routeKey: "PUT /v1/me/ai-consent", authorizer: nil, wantStatus: 401, wantBody: `{"message":"unauthorized"}`},
		{name: "withdraw consent without an authorizer", routeKey: "DELETE /v1/me/ai-consent", authorizer: nil, wantStatus: 401, wantBody: `{"message":"unauthorized"}`},
		{name: "create a group without an authorizer", routeKey: "POST /v1/groups", authorizer: nil, wantStatus: 401, wantBody: `{"message":"unauthorized"}`},
		{name: "read a group without an authorizer", routeKey: "GET /v1/groups/{groupId}", authorizer: nil, wantStatus: 401, wantBody: `{"message":"unauthorized"}`},
		{name: "rename a group without an authorizer", routeKey: "PATCH /v1/groups/{groupId}", authorizer: nil, wantStatus: 401, wantBody: `{"message":"unauthorized"}`},
		{name: "list members without an authorizer", routeKey: "GET /v1/groups/{groupId}/members", authorizer: nil, wantStatus: 401, wantBody: `{"message":"unauthorized"}`},
		{name: "remove a member without an authorizer", routeKey: "DELETE /v1/groups/{groupId}/members/{userId}", authorizer: nil, wantStatus: 401, wantBody: `{"message":"unauthorized"}`},
		{name: "invite without an authorizer", routeKey: "POST /v1/groups/{groupId}/invitations", authorizer: nil, wantStatus: 401, wantBody: `{"message":"unauthorized"}`},
		{name: "list invitations without an authorizer", routeKey: "GET /v1/groups/{groupId}/invitations", authorizer: nil, wantStatus: 401, wantBody: `{"message":"unauthorized"}`},
		{name: "cancel an invitation without an authorizer", routeKey: "DELETE /v1/groups/{groupId}/invitations/{invitationId}", authorizer: nil, wantStatus: 401, wantBody: `{"message":"unauthorized"}`},
		{name: "unknown route", routeKey: "GET /nope", authorizer: jwt(map[string]string{"sub": sub}), wantStatus: 404, wantBody: `{"message":"not found"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := events.APIGatewayV2HTTPRequest{RouteKey: tt.routeKey, RequestContext: events.APIGatewayV2HTTPRequestContext{Authorizer: tt.authorizer}}
			resp, err := (&Server{}).Handle(context.Background(), req)
			if err != nil {
				t.Fatalf("Handle returned error %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if got := resp.Headers["Content-Type"]; got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			if resp.Body != tt.wantBody {
				t.Errorf("body = %s, want %s", resp.Body, tt.wantBody)
			}
		})
	}
}
