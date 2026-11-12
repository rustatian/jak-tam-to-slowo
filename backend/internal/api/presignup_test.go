package api

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

func TestPreSignUp(t *testing.T) {
	const (
		day      = 24 * time.Hour
		signUp   = "PreSignUp_SignUp"
		notFound = "invitation not found"
	)
	withToken := map[string]string{"invitation": tokenRodzina}
	tests := []struct {
		name    string
		row     string            // the invitation with tokenRodzina for nowy@example.com: "group", "new group", "expired" or "none"
		source  string            // the trigger source of the event
		email   string            // the address of the account
		data    map[string]string // the validation data of the sign-up
		closed  bool              // the database pool is closed before the call
		wantErr string            // the text that Cognito sends to the person; empty when the sign-up passes
	}{
		{name: "a pending invitation of a group lets the sign-up pass", row: "group", source: signUp, email: "nowy@example.com", data: withToken},
		{name: "a pending new group invitation lets the sign-up pass", row: "new group", source: signUp, email: "nowy@example.com", data: withToken},
		{name: "an address that differs from the invitation address passes", row: "group", source: signUp, email: "inny@example.com", data: withToken},
		{name: "no token is rejected", row: "group", source: signUp, email: "nowy@example.com", wantErr: notFound},
		{name: "an unknown token is rejected", row: "group", source: signUp, email: "nowy@example.com", data: map[string]string{"invitation": "NOPE"}, wantErr: notFound},
		{name: "an expired invitation is rejected", row: "expired", source: signUp, email: "nowy@example.com", data: withToken, wantErr: notFound},
		{name: "an account that the operator makes needs no token", row: "none", source: "PreSignUp_AdminCreateUser", email: "nowy@example.com"},
		{name: "another trigger source without a token is rejected", row: "group", source: "PreSignUp_ExternalProvider", email: "nowy@example.com", wantErr: notFound},
		{name: "a closed database gives the fixed text", row: "group", source: signUp, email: "nowy@example.com", data: withToken, closed: true, wantErr: "sign-up is not possible now"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool, srv, rodzina, _ := groupFixture(t)
			rows := 1
			switch tt.row {
			case "group":
				invitationRow(t, pool, rodzina, "nowy@example.com", tokenHash(tokenRodzina), nil, -day, 6*day)
			case "new group":
				invitationRow(t, pool, "", "nowy@example.com", tokenHash(tokenRodzina), nil, -day, 6*day)
			case "expired":
				invitationRow(t, pool, rodzina, "nowy@example.com", tokenHash(tokenRodzina), nil, -8*day, -time.Minute)
			case "none":
				rows = 0
			}
			if tt.closed {
				pool.Close()
			}
			event := events.CognitoEventUserPoolsPreSignup{
				CognitoEventUserPoolsHeader: events.CognitoEventUserPoolsHeader{TriggerSource: tt.source, UserName: "1b2c3d4e"},
				Request: events.CognitoEventUserPoolsPreSignupRequest{
					UserAttributes: map[string]string{"email": tt.email},
					ValidationData: tt.data,
				},
			}

			got, err := srv.PreSignUp(ctx, event)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("PreSignUp returned error %v, want none", err)
			case tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr):
				t.Errorf("PreSignUp returned error %v, want %q", err, tt.wantErr)
			}
			// The answer is the event as it came: the function confirms no account and verifies no address.
			if tt.wantErr == "" && !reflect.DeepEqual(got, event) {
				t.Errorf("answer = %+v, want the event %+v", got, event)
			}
			if tt.closed {
				return
			}
			// The check does not use the invitation up.
			if got := countRows(t, pool, "invitations"); got != rows {
				t.Errorf("invitation rows = %d, want %d", got, rows)
			}
		})
	}
}
