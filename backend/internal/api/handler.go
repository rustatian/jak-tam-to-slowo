package api

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../../api/openapi.yaml

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
)

// Handle serves the API Gateway HTTP API routes of the API Lambda.
func Handle(_ context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	switch req.RouteKey {
	case "GET /me":
		return getMe(req), nil
	default:
		return jsonResponse(http.StatusNotFound, Error{Message: "not found"}), nil
	}
}

func getMe(req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	var sub string
	if a := req.RequestContext.Authorizer; a != nil && a.JWT != nil {
		sub = a.JWT.Claims["sub"]
	}
	if sub == "" {
		return jsonResponse(http.StatusUnauthorized, Error{Message: "unauthorized"})
	}
	return jsonResponse(http.StatusOK, Me{Id: sub})
}

// jsonResponse marshals v; the generated contract types always marshal.
func jsonResponse(status int, v any) events.APIGatewayV2HTTPResponse {
	body, _ := json.Marshal(v)
	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(body),
	}
}
