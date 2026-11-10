package db

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/rustatian/jak-tam-to-slowo/backend/internal/dbtest"
)

func TestTx(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name      string
		fnErr     error
		wantCalls int32
		wantErr   func(error) bool
		wantRows  int
	}{
		{name: "commits a successful function", fnErr: nil, wantCalls: 1, wantErr: func(err error) bool { return err == nil }, wantRows: 1},
		{name: "returns another error at once", fnErr: boom, wantCalls: 1, wantErr: func(err error) bool { return errors.Is(err, boom) }, wantRows: 0},
		{name: "gives up after 5 serialization failures", fnErr: &pgconn.PgError{Code: "40001"}, wantCalls: 5, wantErr: isSerializationFailure, wantRows: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pool := dbtest.New(t)
			if _, err := pool.Exec(ctx, "CREATE TABLE items (id int PRIMARY KEY)"); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			err := Tx(ctx, pool, func(tx pgx.Tx) error {
				calls.Add(1)
				if _, err := tx.Exec(ctx, "INSERT INTO items VALUES (1) ON CONFLICT DO NOTHING"); err != nil {
					return err
				}
				return tt.fnErr
			})
			if !tt.wantErr(err) {
				t.Errorf("Tx error = %v", err)
			}
			if got := calls.Load(); got != tt.wantCalls {
				t.Errorf("fn ran %d times, want %d", got, tt.wantCalls)
			}
			var rows int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM items").Scan(&rows); err != nil {
				t.Fatal(err)
			}
			if rows != tt.wantRows {
				t.Errorf("rows = %d, want %d", rows, tt.wantRows)
			}
		})
	}
}

func TestTxRetriesRealConflict(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t)
	for _, sql := range []string{"CREATE TABLE counter (id int PRIMARY KEY, n int NOT NULL)", "INSERT INTO counter VALUES (1, 0)"} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	var barrier sync.WaitGroup
	barrier.Add(2)
	var calls atomic.Int32
	increment := func() error {
		first := true
		return Tx(ctx, pool, func(tx pgx.Tx) error {
			calls.Add(1)
			var n int
			if err := tx.QueryRow(ctx, "SELECT n FROM counter WHERE id = 1").Scan(&n); err != nil {
				return err
			}
			if first {
				first = false
				barrier.Done()
				barrier.Wait()
			}
			_, err := tx.Exec(ctx, "UPDATE counter SET n = $1 WHERE id = 1", n+1)
			return err
		})
	}
	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- increment() }()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT n FROM counter WHERE id = 1").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("n = %d, want 2", n)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("fn ran %d times, want 3 (one retry)", got)
	}
}

func TestPoolConfig(t *testing.T) {
	cfg, err := poolConfig(Config{Endpoint: "x.dsql.eu-central-1.on.aws", Region: "eu-central-1", User: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConnLifetime != 55*time.Minute {
		t.Errorf("MaxConnLifetime = %v, want 55m", cfg.MaxConnLifetime)
	}
	if cfg.ConnConfig.User != "app" || cfg.ConnConfig.Host != "x.dsql.eu-central-1.on.aws" || cfg.ConnConfig.Database != "postgres" {
		t.Errorf("conn config = %s@%s/%s", cfg.ConnConfig.User, cfg.ConnConfig.Host, cfg.ConnConfig.Database)
	}
	if cfg.ConnConfig.TLSConfig == nil || cfg.ConnConfig.TLSConfig.ServerName != "x.dsql.eu-central-1.on.aws" {
		t.Error("TLS must verify the endpoint name (sslmode=verify-full)")
	}
	if cfg.BeforeConnect == nil {
		t.Error("BeforeConnect is nil; each new connection needs a fresh IAM token")
	}
}
