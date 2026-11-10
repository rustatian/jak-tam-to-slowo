// Package db connects to Aurora DSQL and runs transactions with the retry that its optimistic concurrency needs.
package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dsql/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config selects a DSQL cluster and the database role. User "admin" uses the admin token.
type Config struct {
	Endpoint    string
	Region      string
	User        string
	Credentials aws.CredentialsProvider
}

// Open returns a pool for DSQL. No connection opens until the first query.
func Open(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	pc, err := poolConfig(cfg)
	if err != nil {
		return nil, err
	}
	return pgxpool.NewWithConfig(ctx, pc)
}

// OpenURL returns a pool for a PostgreSQL connection string.
func OpenURL(ctx context.Context, url string) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, url)
}

func poolConfig(cfg Config) (*pgxpool.Config, error) {
	pc, err := pgxpool.ParseConfig(fmt.Sprintf("host=%s port=5432 user=%s dbname=postgres sslmode=verify-full", cfg.Endpoint, cfg.User))
	if err != nil {
		return nil, err
	}
	// DSQL ends a connection after 60 minutes.
	// https://docs.aws.amazon.com/aurora-dsql/latest/userguide/authentication-authorization.html
	pc.MaxConnLifetime = 55 * time.Minute
	token := auth.GenerateDbConnectAuthToken
	if cfg.User == "admin" {
		token = auth.GenerateDBConnectAdminAuthToken
	}
	// A token is checked only when a connection opens, so each new connection gets a fresh one.
	pc.BeforeConnect = func(ctx context.Context, cc *pgx.ConnConfig) error {
		t, err := token(ctx, cfg.Endpoint, cfg.Region, cfg.Credentials)
		if err != nil {
			return fmt.Errorf("dsql auth token: %w", err)
		}
		cc.Password = t
		return nil
	}
	return pc, nil
}

const maxAttempts = 5

// Tx runs fn in a transaction, and runs it again while the commit fails with SQLSTATE 40001 (OC000, OC001).
// https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-concurrency-control.html
func Tx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = pgx.BeginFunc(ctx, pool, fn)
		if !isSerializationFailure(err) || attempt == maxAttempts {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 20 * time.Millisecond):
		}
	}
	return err
}

func isSerializationFailure(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "40001"
}
