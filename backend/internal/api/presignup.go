package api

import (
	"context"
	"errors"
	"log/slog"

	"github.com/aws/aws-lambda-go/events"
)

// PreSignUp is the pre sign-up trigger of the user pool. A sign-up passes only with the token of a pending
// invitation in its validation data, under the name invitation. The token is the proof, so the function reads no
// other part of the request, and it does not use the invitation up. An account that the operator makes needs no
// token. Cognito sends the text of a returned error to the person who signs up.
// https://docs.aws.amazon.com/cognito/latest/developerguide/cognito-user-pools-working-with-lambda-triggers.html#important-lambda-considerations
func (s *Server) PreSignUp(ctx context.Context, event events.CognitoEventUserPoolsPreSignup) (events.CognitoEventUserPoolsPreSignup, error) {
	if event.TriggerSource == "PreSignUp_AdminCreateUser" {
		return event, nil
	}
	var pending bool
	err := s.DB.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM invitations WHERE token_hash = $1 AND expires_at > now())",
		tokenHash(event.Request.ValidationData["invitation"])).Scan(&pending)
	if err != nil {
		// The log gets the cause and no part of the event, because the event has the address and the token.
		slog.ErrorContext(ctx, "pre sign-up", "error", err)
		return event, errors.New("sign-up is not possible now")
	}
	if !pending {
		return event, errors.New("invitation not found")
	}
	return event, nil
}
