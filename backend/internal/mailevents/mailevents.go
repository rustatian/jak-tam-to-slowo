// Package mailevents records what Amazon SES reports about the invitation emails, and deletes the expired
// invitations on a schedule.
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

// Handler receives the events of the invitation emails from Amazon EventBridge, and the event of the schedule.
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

// Handle marks the invitation of a bounce or a complaint event, and deletes the expired invitations on the event
// of the schedule. It writes no part of an event into the log, because a mail event has the address of the
// invited person.
func (h *Handler) Handle(ctx context.Context, event events.EventBridgeEvent) error {
	var failure string
	switch event.DetailType {
	case "Scheduled Event":
		// shortcut: one transaction deletes all expired invitations, and Aurora DSQL lets a transaction change
		// 3,000 rows. Delete in parts when more than 150 members can each send their 20 invitations on one day.
		// https://docs.aws.amazon.com/aurora-dsql/latest/userguide/CHAP_quotas.html#SECTION_database-limits
		return db.Tx(ctx, h.DB, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "DELETE FROM invitations WHERE accepted_at IS NULL AND expires_at <= now()")
			return err
		})
	case "Email Bounced":
		// Every bounce counts, also a transient one. An automatic reply of the receiver can cause a transient
		// bounce for an email that arrived, and the event does not tell the two apart.
		// https://docs.aws.amazon.com/ses/latest/dg/notification-contents.html#bounce-types
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
	// https://www.postgresql.org/docs/16/errcodes-appendix.html
	return db.Tx(ctx, h.DB, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE invitations SET failure = $1 WHERE id::text = $2", failure, ids[0])
		return err
	})
}
