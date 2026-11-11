// Package mailevents records what Amazon SES reports about the invitation emails.
package mailevents

import (
	"context"
	"encoding/json/v2"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
)

// Handler receives the events of the invitation emails from Amazon EventBridge.
type Handler struct {
	DB *pgxpool.Pool
}

// detail is the part of an event that names the email. The API sends each invitation email with the message tag
// "invitation", whose value is the ID of the invitation. A tag has a list of values.
// https://docs.aws.amazon.com/ses/latest/dg/monitoring-eventbridge.html
type detail struct {
	Mail struct {
		Tags map[string][]string `json:"tags"`
	} `json:"mail"`
}

// Handle marks the invitation of a bounce or a complaint event. It writes no part of the event into the log,
// because an event has the address of the invited person.
func (h *Handler) Handle(ctx context.Context, event events.EventBridgeEvent) error {
	var failure string
	switch event.DetailType {
	case "Email Bounced":
		failure = "bounce"
	case "Email Complaint Received":
		failure = "complaint"
	default:
		return fmt.Errorf("unexpected detail type %q", event.DetailType)
	}
	var d detail
	if err := json.Unmarshal(event.Detail, &d); err != nil {
		return err
	}
	ids := d.Mail.Tags["invitation"]
	if len(ids) == 0 {
		return nil
	}
	// The invitation can be gone, so zero changed rows is no error. The ID is compared as text: PostgreSQL
	// refuses a uuid parameter that is not a UUID (SQLSTATE 22P02).
	return db.Tx(ctx, h.DB, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE invitations SET failure = $1 WHERE id::text = $2", failure, ids[0])
		return err
	})
}
