package postgres

/*
MIT License

Copyright (c) 2026 Shane

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
*/

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SQLSTATE codes that RunInTx treats as retryable: a serializable transaction
// that lost a conflict (40001) or a transaction chosen as a deadlock victim
// (40P01). Both are transient and safe to retry from the top.
const (
	sqlStateSerializationFailure = "40001"
	sqlStateDeadlockDetected     = "40P01"
)

// DB is a thin, resilient wrapper over a pgx connection pool. Build one with
// New; use Pool to reach the full pgx API, RunInTx to run a transaction with
// automatic serialization-failure retries, Healthy for a liveness probe, and
// Close to release the pool. A DB is safe for concurrent use by multiple
// goroutines.
type DB struct {
	pool  *pgxpool.Pool
	retry retryConfig
}

// txBeginner is the slice of the pool RunInTx needs. Depending on it (rather than
// the concrete *pgxpool.Pool) lets the retry loop be exercised with a mock pool.
type txBeginner interface {
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
}

// New parses the DSN, builds a pgx connection pool with the resilient defaults
// for anything left unset, and verifies readiness with a Ping before returning.
// The DSN accepts either a keyword/value string or a URL, for example
// "postgres://user:pass@localhost:5432/acme". On a failed Ping the pool is closed
// and the error is returned, so a returned DB is always usable. The ctx bounds
// both the pool creation and the readiness Ping.
func New(ctx context.Context, dsn string, opts ...Option) (*DB, error) {
	cfg := defaults()
	for _, opt := range opts {
		opt(&cfg)
	}

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse dsn: %w", err)
	}
	cfg.applyTo(poolCfg)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping on connect: %w", err)
	}
	return &DB{pool: pool, retry: cfg.retry}, nil
}

// Pool returns the underlying pgx pool so callers can use the full pgx API
// (Query, Exec, CopyFrom, LISTEN/NOTIFY, and so on). The returned value is owned
// by this DB; do not Close it directly -- use DB.Close.
func (db *DB) Pool() *pgxpool.Pool { return db.pool }

// Ping verifies a round-trip to the server within ctx, acquiring and releasing a
// pooled connection. It returns the underlying error on failure.
func (db *DB) Ping(ctx context.Context) error {
	if err := db.pool.Ping(ctx); err != nil {
		return fmt.Errorf("postgres: ping: %w", err)
	}
	return nil
}

// Healthy reports whether the server answers a Ping within ctx. It is cheap and
// suitable for readiness and liveness probes.
func (db *DB) Healthy(ctx context.Context) bool {
	return db.pool.Ping(ctx) == nil
}

// Close releases the connection pool and waits for in-flight queries to finish.
// It is safe to call once; further use of the DB after Close returns errors from
// pgx.
func (db *DB) Close() { db.pool.Close() }

// RunInTx runs fn inside a transaction and commits it. If fn or the commit fails
// with a serialization failure (SQLSTATE 40001) or a deadlock (40P01), the whole
// transaction is retried from the top with capped exponential backoff, up to the
// configured attempt limit. Any other error -- including an application error
// returned by fn -- aborts immediately without a retry. fn must be idempotent
// across attempts: it may run more than once, and it must not commit or roll back
// the transaction itself. The default policy comes from WithTxRetry; per-call
// TxOptions override it and set the pgx transaction options (for example
// Serializable isolation, where retries are expected under contention).
func (db *DB) RunInTx(ctx context.Context, fn func(pgx.Tx) error, opts ...TxOption) error {
	txc := txConfig{retry: db.retry}
	for _, opt := range opts {
		opt(&txc)
	}
	return runInTx(ctx, db.pool, txc, fn)
}

// runInTx is the pool-agnostic retry loop behind RunInTx.
func runInTx(ctx context.Context, b txBeginner, txc txConfig, fn func(pgx.Tx) error) error {
	var err error
	for attempt := 1; ; attempt++ {
		err = runOnce(ctx, b, txc.txOpts, fn)
		if err == nil {
			return nil
		}
		if !Retryable(err) {
			return err
		}
		if attempt >= txc.retry.attempts {
			return fmt.Errorf("postgres: transaction aborted after %d attempts: %w", txc.retry.attempts, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff(attempt, txc.retry.baseBackoff, txc.retry.maxBackoff)):
		}
	}
}

// runOnce executes a single transaction attempt: begin, run fn, commit. A
// deferred Rollback is a no-op after a successful Commit (pgx returns ErrTxClosed,
// which is ignored) and unwinds the transaction on any early return.
func runOnce(ctx context.Context, b txBeginner, txo pgx.TxOptions, fn func(pgx.Tx) error) error {
	tx, err := b.BeginTx(ctx, txo)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Retryable reports whether err is a transient PostgreSQL failure that RunInTx
// retries: a serialization failure (SQLSTATE 40001) or a deadlock (40P01). It
// unwraps the error chain, so a wrapped *pgconn.PgError is still recognized.
func Retryable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == sqlStateSerializationFailure || pgErr.Code == sqlStateDeadlockDetected
	}
	return false
}

// backoff returns the delay before the given retry attempt (1-based) as capped
// exponential growth: base, 2*base, 4*base, ..., clamped at max. The doubling is
// guarded so a large attempt count cannot overflow the duration.
func backoff(attempt int, base, max time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := base
	for i := 1; i < attempt; i++ {
		d *= 2
		if d <= 0 || d >= max {
			return max
		}
	}
	if d > max {
		return max
	}
	return d
}
