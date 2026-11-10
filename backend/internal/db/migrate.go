package db

import (
	"context"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate applies the *.sql files of files in name order and records each version in schema_migrations.
// Each file holds one statement and runs in its own transaction: DSQL allows one DDL statement per
// transaction and no DDL and DML together.
// https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-ddl.html
func Migrate(ctx context.Context, pool *pgxpool.Pool, files fs.FS) ([]string, error) {
	// One connection for all statements: another connection can hold the catalog from before a DDL statement.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())"); err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	var done []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		done = append(done, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return nil, err
	}
	slices.Sort(names)
	var applied []string
	for _, name := range names {
		version := strings.TrimSuffix(name, ".sql")
		if slices.Contains(done, version) {
			continue
		}
		b, err := fs.ReadFile(files, name)
		if err != nil {
			return applied, err
		}
		sql := strings.TrimSpace(string(b))
		if strings.Contains(strings.TrimSuffix(sql, ";"), ";") {
			return applied, fmt.Errorf("%s: one statement per file", name)
		}
		if _, err := conn.Exec(ctx, sql); err != nil {
			return applied, fmt.Errorf("%s: %w", name, err)
		}
		if _, err := conn.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
			return applied, fmt.Errorf("%s: record version: %w", name, err)
		}
		applied = append(applied, version)
	}
	return applied, nil
}

// roleARN matches an IAM role ARN; the value goes into SQL text, so it allows no quote.
var roleARN = regexp.MustCompile(`^arn:aws:iam::[0-9]{12}:role/[A-Za-z0-9+=,.@_/-]+$`)

// SetupAppRole makes the DSQL database role app, maps it to the IAM role arn, and lets it read and write
// the tables but not schema_migrations. Each statement runs in its own transaction, and a second run changes nothing.
// https://docs.aws.amazon.com/aurora-dsql/latest/userguide/using-database-and-iam-roles.html
func SetupAppRole(ctx context.Context, pool *pgxpool.Pool, arn string) error {
	if !roleARN.MatchString(arn) {
		return fmt.Errorf("invalid IAM role ARN %q", arn)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	var exists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app')").Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err := conn.Exec(ctx, "CREATE ROLE app WITH LOGIN"); err != nil {
			return err
		}
	}
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM sys.iam_pg_role_mappings WHERE pg_role_name = 'app' AND arn = $1)", arn).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err := conn.Exec(ctx, "AWS IAM GRANT app TO '"+arn+"'"); err != nil {
			return err
		}
	}
	for _, sql := range []string{
		"GRANT USAGE ON SCHEMA public TO app",
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app",
		"REVOKE ALL ON schema_migrations FROM app",
	} {
		if _, err := conn.Exec(ctx, sql); err != nil {
			return fmt.Errorf("%s: %w", sql, err)
		}
	}
	return nil
}
