package d1import

import (
	"context"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/db"
	"github.com/rustatian/jak-tam-to-slowo/backend/internal/dbtest"
	"github.com/rustatian/jak-tam-to-slowo/backend/migrations"
)

// The old users O1 and O2 and the translation cards K1 and K2 of the export in testdata. The export also has the
// grammar card K3.
const (
	o1 = "0000000a-0000-4000-8000-000000000001"
	o2 = "0000000a-0000-4000-8000-000000000002"
	k1 = "0000000c-0000-4000-8000-000000000001"
	k2 = "0000000c-0000-4000-8000-000000000002"
)

func TestRun(t *testing.T) {
	base := map[string]string{o1: "n1", o2: "n2"}
	report := Report{Cards: 2, Grammar: 1, Progress: map[string]int{o1: 2, o2: 1}}
	tests := []struct {
		name         string
		setup        string            // SQL after the fixture of the database, or empty
		users        map[string]string // the mapping of the old user IDs
		write        bool
		wantErr      string // a part of the text of the error, empty for no error
		wantReport   Report
		wantCards    int // the rows of cards after the call
		wantProgress int // the rows of card_progress after the call
	}{
		{name: "writes the cards and the progress rows", users: base, write: true, wantReport: report, wantCards: 2, wantProgress: 3},
		{name: "a dry run reports the same counts and writes nothing", users: base, wantReport: report},
		{name: "a dry run does the inserts", users: base, wantErr: "duplicate key", wantCards: 1,
			setup: `INSERT INTO cards (group_id, id, pl, ru, example_pl, example_ru, meaning_ru) SELECT id, '` + k1 + `', 'las', 'лес', 'To jest las.', 'Это лес.', 'деревья' FROM groups WHERE name = 'Praca'`},
		{name: "a group that has a card is refused", users: base, write: true, wantErr: "the group already has 1 cards", wantCards: 1,
			setup: `INSERT INTO cards (group_id, pl, ru, example_pl, example_ru, meaning_ru) SELECT id, 'las', 'лес', 'To jest las.', 'Это лес.', 'деревья' FROM groups WHERE name = 'Rodzina'`},
		{name: "an old user with progress and no account is refused", users: map[string]string{o1: "n1"}, write: true, wantErr: "no account for the old user " + o2},
		{name: "an account that is not a member of the group is refused", users: map[string]string{o1: "n1", o2: "n3"}, write: true, wantErr: "the account n3 is not a member of the group"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool := dbtest.New(t)
			if _, err := db.Migrate(ctx, pool, migrations.FS); err != nil {
				t.Fatal(err)
			}
			var rodzina, praca string
			if err := pool.QueryRow(ctx, "INSERT INTO groups (name) VALUES ('Rodzina') RETURNING id").Scan(&rodzina); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, "INSERT INTO groups (name) VALUES ('Praca') RETURNING id").Scan(&praca); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, "INSERT INTO users (id, email, group_id) VALUES ('n1', 'n1@example.com', $1), ('n2', 'n2@example.com', $1), ('n3', 'n3@example.com', $2)",
				rodzina, praca); err != nil {
				t.Fatal(err)
			}
			if tt.setup != "" {
				if _, err := pool.Exec(ctx, tt.setup); err != nil {
					t.Fatal(err)
				}
			}

			got, err := Run(ctx, pool, os.DirFS("testdata"), Options{Group: rodzina, Users: tt.users, Write: tt.write})
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("Run returned error %v, want none", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Errorf("Run returned error %v, want one with %q", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.wantReport) {
				t.Errorf("Run returned the report %+v, want %+v", got, tt.wantReport)
			}
			var cards, progress int
			if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM cards), (SELECT count(*) FROM card_progress)").Scan(&cards, &progress); err != nil {
				t.Fatal(err)
			}
			if cards != tt.wantCards || progress != tt.wantProgress {
				t.Errorf("the tables have %d cards and %d progress rows, want %d and %d", cards, progress, tt.wantCards, tt.wantProgress)
			}
			if !tt.write || tt.wantErr != "" {
				return
			}

			// A run that writes stores the values of the export. Each time is compared as its instant in UTC.
			utc := func(p *time.Time) string {
				if p == nil {
					return "NULL"
				}
				return p.UTC().Format(time.RFC3339Nano)
			}
			old := func(ms int64) string { return utc(new(time.UnixMilli(ms))) }

			type cardRow struct {
				id, group, kind, pl, ru, examplePl, exampleRu, meaningRu string
				altPl, altRu                                             string // the text of the jsonb value
				archived                                                 bool
				createdAt                                                string
			}
			rows, err := pool.Query(ctx, "SELECT id, group_id, kind, pl, ru, example_pl, example_ru, meaning_ru, alternatives_pl::text, alternatives_ru::text, archived, created_at FROM cards ORDER BY id")
			if err != nil {
				t.Fatal(err)
			}
			gotCards, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (cardRow, error) {
				var c cardRow
				var created time.Time
				err := row.Scan(&c.id, &c.group, &c.kind, &c.pl, &c.ru, &c.examplePl, &c.exampleRu, &c.meaningRu, &c.altPl, &c.altRu, &c.archived, &created)
				c.createdAt = utc(&created)
				return c, err
			})
			if err != nil {
				t.Fatal(err)
			}
			// The jsonb text has the form of the PostgreSQL output, with a space after each comma.
			// https://www.postgresql.org/docs/16/datatype-json.html
			// No card has the ID of K3, the grammar card.
			wantCards := []cardRow{
				{id: k1, group: rodzina, kind: "translation", pl: "dom", ru: "дом", examplePl: "To jest dom.", exampleRu: "Это дом.", meaningRu: "здание",
					altPl: `["domek", "chata"]`, altRu: `["жилище"]`, createdAt: old(1790000000000)},
				{id: k2, group: rodzina, kind: "translation", pl: "kot", ru: "кот", examplePl: "To jest kot.", exampleRu: "Это кот.", meaningRu: "животное",
					altPl: `["kotek"]`, altRu: `[]`, archived: true, createdAt: old(1790000000001)},
			}
			if !slices.Equal(gotCards, wantCards) {
				t.Errorf("the card rows are\n%+v\nwant\n%+v", gotCards, wantCards)
			}

			type progressRow struct {
				user, card                 string
				correct, incorrect         int
				lastCorrect, lastIncorrect string
				version                    int
			}
			rows, err = pool.Query(ctx, "SELECT user_id, card_id, correct, incorrect, last_correct_at, last_incorrect_at, version FROM card_progress ORDER BY user_id, card_id")
			if err != nil {
				t.Fatal(err)
			}
			gotProgress, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (progressRow, error) {
				var p progressRow
				var lastCorrect, lastIncorrect *time.Time
				err := row.Scan(&p.user, &p.card, &p.correct, &p.incorrect, &lastCorrect, &lastIncorrect, &p.version)
				p.lastCorrect, p.lastIncorrect = utc(lastCorrect), utc(lastIncorrect)
				return p, err
			})
			if err != nil {
				t.Fatal(err)
			}
			wantProgress := []progressRow{
				{"n1", k1, 3, 1, old(1794300000000), old(1794000000000), 4},
				{"n1", k2, 0, 2, "NULL", old(1794100000000), 2},
				{"n2", k1, 1, 0, old(1794200000000), "NULL", 1},
			}
			if !slices.Equal(gotProgress, wantProgress) {
				t.Errorf("the progress rows are\n%+v\nwant\n%+v", gotProgress, wantProgress)
			}
		})
	}
}
