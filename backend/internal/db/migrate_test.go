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
