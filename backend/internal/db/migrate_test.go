package db

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/dbtest"
	"github.com/rustatian/jak-tam-to-slowo/backend/migrations"
)

func sqlFiles(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, sql := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(sql)}
	}
	return fsys
}

func TestMigrate(t *testing.T) {
	ab := map[string]string{"0002_b.sql": "CREATE TABLE b (id int)", "0001_a.sql": "CREATE TABLE a (id int);\n"}
	tests := []struct {
		name         string
		files        map[string]string
		runs         int
		wantApplied  []string
		wantErr      string
		wantVersions []string
	}{
		{name: "applies files in name order", files: ab, runs: 1, wantApplied: []string{"0001_a", "0002_b"}, wantVersions: []string{"0001_a", "0002_b"}},
		{name: "second run applies nothing", files: ab, runs: 2, wantApplied: nil, wantVersions: []string{"0001_a", "0002_b"}},
		{name: "a failed file keeps earlier versions", files: map[string]string{"0001_a.sql": "CREATE TABLE a (id int)", "0002_bad.sql": "CREATE TABLE"}, runs: 1, wantApplied: []string{"0001_a"}, wantErr: "0002_bad.sql", wantVersions: []string{"0001_a"}},
		{name: "rejects a file with two statements", files: map[string]string{"0001_two.sql": "CREATE TABLE a (id int); CREATE TABLE b (id int);"}, runs: 1, wantApplied: nil, wantErr: "0001_two.sql: one statement per file", wantVersions: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool := dbtest.New(t)
			var applied []string
			var err error
			for range tt.runs {
				applied, err = Migrate(ctx, pool, sqlFiles(tt.files))
			}
			if tt.wantErr == "" && err != nil {
				t.Fatalf("Migrate: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("Migrate error = %v, want it to contain %q", err, tt.wantErr)
			}
			if !slices.Equal(applied, tt.wantApplied) {
				t.Errorf("applied = %v, want %v", applied, tt.wantApplied)
			}
			rows, err := pool.Query(ctx, "SELECT version FROM schema_migrations ORDER BY version")
			if err != nil {
				t.Fatal(err)
			}
			var versions []string
			for rows.Next() {
				var v string
				if err := rows.Scan(&v); err != nil {
					t.Fatal(err)
				}
				versions = append(versions, v)
			}
			if !slices.Equal(versions, tt.wantVersions) {
				t.Errorf("schema_migrations = %v, want %v", versions, tt.wantVersions)
			}
		})
	}
}

func TestMigrateRealFiles(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t)
	if _, err := Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, "INSERT INTO users (id, email, direction) VALUES ('u1', 'a@example.com', 'xx')")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Errorf("direction xx: error = %v, want a check violation (23514)", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO users (id, email) VALUES ('u2', 'b@example.com')"); err != nil {
		t.Fatal(err)
	}
	var direction, displayName string
	if err := pool.QueryRow(ctx, "SELECT direction, display_name FROM users WHERE id = 'u2'").Scan(&direction, &displayName); err != nil {
		t.Fatal(err)
	}
	if direction != "ru-pl" || displayName != "" {
		t.Errorf("defaults = %q, %q; want ru-pl and an empty display name", direction, displayName)
	}
	_, err = pool.Exec(ctx, "INSERT INTO users (id, email, group_id) VALUES ('u3', 'c@example.com', gen_random_uuid())")
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Errorf("a group that does not exist: error = %v, want a foreign key violation (23503)", err)
	}

	var groupID string
	if err := pool.QueryRow(ctx, "INSERT INTO groups (name) VALUES ('g') RETURNING id").Scan(&groupID); err != nil {
		t.Fatal(err)
	}
	const invite = "INSERT INTO invitations (group_id, email, token_hash, expires_at) VALUES ($1, 'a@example.com', 'h', now() + interval '7 days')"
	if _, err := pool.Exec(ctx, invite, groupID); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, invite, groupID)
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Errorf("a second invitation with the same token hash: error = %v, want a unique violation (23505)", err)
	}

	var groupID2 string
	if err := pool.QueryRow(ctx, "INSERT INTO groups (name) VALUES ('g2') RETURNING id").Scan(&groupID2); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, "INSERT INTO cards (group_id, kind, pl, ru, example_pl, example_ru, meaning_ru) VALUES ($1, 'x', 'dom', 'дом', 'To jest dom.', 'Это дом.', 'здание')", groupID)
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Errorf("card kind x: error = %v, want a check violation (23514)", err)
	}
	var cardID string
	if err := pool.QueryRow(ctx, "INSERT INTO cards (group_id, pl, ru, example_pl, example_ru, meaning_ru) VALUES ($1, 'dom', 'дом', 'To jest dom.', 'Это дом.', 'здание') RETURNING id", groupID).Scan(&cardID); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, "INSERT INTO cards (group_id, id, pl, ru, example_pl, example_ru, meaning_ru) VALUES ($1, $2, 'dom', 'дом', 'To jest dom.', 'Это дом.', 'здание')", groupID2, cardID)
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Errorf("the same card ID in a second group: error = %v, want a unique violation (23505)", err)
	}
	const progress = "INSERT INTO card_progress (user_id, card_id, correct, incorrect, version) VALUES ('u2', $1, 1, 0, 1)"
	if _, err := pool.Exec(ctx, progress, cardID); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, progress, cardID)
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Errorf("a second progress row of the same learner and card: error = %v, want a unique violation (23505)", err)
	}
	_, err = pool.Exec(ctx, "DELETE FROM users WHERE id = 'u2'")
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Errorf("the deletion of a user with a progress row: error = %v, want a foreign key violation (23503)", err)
	}
	const review = "INSERT INTO reviews (user_id, attempt_id, card_id, grade) VALUES ('u2', '00000000-0000-0000-0000-0000000000a1', $1, 'correct')"
	if _, err := pool.Exec(ctx, review, cardID); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, review, cardID)
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Errorf("a second review of the same learner and attempt: error = %v, want a unique violation (23505)", err)
	}
	_, err = pool.Exec(ctx, "INSERT INTO reviews (user_id, attempt_id, card_id, grade) VALUES ('u2', '00000000-0000-0000-0000-0000000000a2', $1, 'x')", cardID)
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Errorf("review grade x: error = %v, want a check violation (23514)", err)
	}

	if _, err := pool.Exec(ctx, "DELETE FROM groups WHERE id = $1", groupID); err != nil {
		t.Fatal(err)
	}
	var invitations int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM invitations").Scan(&invitations); err != nil {
		t.Fatal(err)
	}
	if invitations != 0 {
		t.Errorf("invitations after the deletion of their group = %d, want 0", invitations)
	}
	var cards int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM cards").Scan(&cards); err != nil {
		t.Fatal(err)
	}
	if cards != 0 {
		t.Errorf("cards after the deletion of their group = %d, want 0", cards)
	}
	var progressRows int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM card_progress").Scan(&progressRows); err != nil {
		t.Fatal(err)
	}
	if progressRows != 0 {
		t.Errorf("progress rows after the deletion of the group of their card = %d, want 0", progressRows)
	}
	var reviews int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM reviews").Scan(&reviews); err != nil {
		t.Fatal(err)
	}
	if reviews != 1 {
		t.Errorf("reviews after the deletion of the group of their card = %d, want 1", reviews)
	}
	_, err = pool.Exec(ctx, "DELETE FROM users WHERE id = 'u2'")
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Errorf("the deletion of a user with a review: error = %v, want a foreign key violation (23503)", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM reviews WHERE user_id = 'u2'"); err != nil {
		t.Fatal(err)
	}

	_, err = pool.Exec(ctx, "INSERT INTO jobs (user_id, kind, status, input) VALUES ('u2', 'draft', 'x', '{}')")
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Errorf("job status x: error = %v, want a check violation (23514)", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO jobs (user_id, kind, input) VALUES ('u2', 'draft', '{}')"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = 'u2'"); err != nil {
		t.Fatal(err)
	}
	var jobs int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs").Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Errorf("jobs after the deletion of their user = %d, want 0", jobs)
	}
}

func TestSetupAppRoleRejectsBadARN(t *testing.T) {
	tests := []struct {
		name string
		arn  string
	}{
		{name: "rejects a quote", arn: "arn:aws:iam::860334207583:role/a'b"},
		{name: "rejects a user ARN", arn: "arn:aws:iam::860334207583:user/x"},
		{name: "rejects a short account", arn: "arn:aws:iam::123:role/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetupAppRole(context.Background(), nil, tt.arn)
			if err == nil || !strings.Contains(err.Error(), "invalid IAM role ARN") {
				t.Errorf("SetupAppRole(%q) error = %v, want an invalid ARN error", tt.arn, err)
			}
		})
	}
}
