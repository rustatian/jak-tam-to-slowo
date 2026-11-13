// Package d1import writes the cards and the progress rows of the D1 export of the old app into one group.
package d1import

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
)

// Options selects the target of an import.
type Options struct {
	Group string            // the ID of the target group
	Users map[string]string // the new user ID for each old user ID
	Write bool              // false rolls the transaction back after the inserts
}

// Report counts the rows of an import: Cards the translation cards written, Grammar the grammar cards left out, and
// Progress the progress rows written for each old user ID.
type Report struct {
	Cards, Grammar int
	Progress       map[string]int
}

// card is a row of cards.json with the columns that the import reads. Each list is a text that holds a JSON array.
type card struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Pl             string `json:"pl"`
	Ru             string `json:"ru"`
	ExamplePl      string `json:"example_pl"`
	ExampleRu      string `json:"example_ru"`
	MeaningRu      string `json:"meaning_ru"`
	AlternativesPl string `json:"alternatives_pl"`
	AlternativesRu string `json:"alternatives_ru"`
	Archived       int    `json:"archived"`
	CreatedAt      int64  `json:"created_at"`
}

// progress is a row of card_progress.json. A time is in milliseconds since 1970, or nil.
type progress struct {
	UserID          string `json:"user_id"`
	CardID          string `json:"card_id"`
	Correct         int    `json:"correct"`
	Incorrect       int    `json:"incorrect"`
	LastCorrectAt   *int64 `json:"last_correct_at"`
	LastIncorrectAt *int64 `json:"last_incorrect_at"`
	Version         int    `json:"version"`
}

// errDryRun ends the transaction of a dry run after the inserts, so that it rolls back.
var errDryRun = errors.New("dry run")

// Run reads cards.json and card_progress.json from files. In one transaction it writes the translation
// cards into the group opts.Group and each progress row to the new user ID of its old user. The grammar cards are
// left out. With an error, the report is the zero Report.
func Run(ctx context.Context, pool *pgxpool.Pool, files fs.FS, opts Options) (Report, error) {
	cards, err := readRows[card](files, "cards.json")
	if err != nil {
		return Report{}, err
	}
	rows, err := readRows[progress](files, "card_progress.json")
	if err != nil {
		return Report{}, err
	}

	// The report and the values of the inserts come from the files here, because db.Tx can run its function again.
	report := Report{Progress: map[string]int{}}
	var cardArgs, progressArgs [][]any
	for _, c := range cards {
		if c.Kind == "grammar" {
			report.Grammar++
			continue
		}
		// pgx sends a string to a jsonb column as the JSON text itself, so each list stays the array of the export.
		// https://github.com/jackc/pgx/blob/v5.12.0/pgtype/json.go#L26
		cardArgs = append(cardArgs, []any{opts.Group, c.ID, c.Pl, c.Ru, c.ExamplePl, c.ExampleRu, c.MeaningRu,
			c.AlternativesPl, c.AlternativesRu, c.Archived == 1, time.UnixMilli(c.CreatedAt)})
	}
	report.Cards = len(cardArgs)
	for _, p := range rows {
		user, ok := opts.Users[p.UserID]
		if !ok {
			return Report{}, fmt.Errorf("no account for the old user %s", p.UserID)
		}
		report.Progress[p.UserID]++
		progressArgs = append(progressArgs, []any{user, p.CardID, p.Correct, p.Incorrect, oldTime(p.LastCorrectAt), oldTime(p.LastIncorrectAt), p.Version})
	}

	// shortcut: all rows go into one transaction, and Aurora DSQL lets one transaction change 3,000 rows. Insert in
	// parts before an export has more cards and progress rows than that.
	// https://docs.aws.amazon.com/aurora-dsql/latest/userguide/CHAP_quotas.html#SECTION_database-limits
	err = db.Tx(ctx, pool, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM cards WHERE group_id = $1", opts.Group).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("the group already has %d cards", n)
		}
		for _, id := range slices.Sorted(maps.Values(opts.Users)) {
			var member bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND group_id = $2)", id, opts.Group).Scan(&member); err != nil {
				return err
			}
			if !member {
				return fmt.Errorf("the account %s is not a member of the group", id)
			}
		}
		for _, args := range cardArgs {
			if _, err := tx.Exec(ctx, "INSERT INTO cards (group_id, id, pl, ru, example_pl, example_ru, meaning_ru, alternatives_pl, alternatives_ru, archived, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)",
				args...); err != nil {
				return err
			}
		}
		for _, args := range progressArgs {
			if _, err := tx.Exec(ctx, "INSERT INTO card_progress (user_id, card_id, correct, incorrect, last_correct_at, last_incorrect_at, version) VALUES ($1, $2, $3, $4, $5, $6, $7)",
				args...); err != nil {
				return err
			}
		}
		if !opts.Write {
			return errDryRun
		}
		return nil
	})
	if err != nil && !errors.Is(err, errDryRun) {
		return Report{}, err
	}
	return report, nil
}

// readRows decodes a file in the output form of wrangler: a JSON array with one object, whose member results holds
// the rows.
func readRows[T any](files fs.FS, name string) ([]T, error) {
	raw, err := fs.ReadFile(files, name)
	if err != nil {
		return nil, err
	}
	var out []struct {
		Results []T `json:"results"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%s is not in the wrangler form: %w", name, err)
	}
	if len(out) != 1 || out[0].Results == nil {
		return nil, fmt.Errorf("%s is not in the wrangler form", name)
	}
	return out[0].Results, nil
}

// oldTime converts a time of the export. A nil time stays nil, which pgx writes as NULL.
func oldTime(ms *int64) *time.Time {
	if ms == nil {
		return nil
	}
	return new(time.UnixMilli(*ms))
}
