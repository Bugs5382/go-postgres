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
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v4"
)

// noopTracer is a pgx QueryTracer that records nothing; it exercises the
// WithTracer seam without a telemetry dependency.
type noopTracer struct{}

func (noopTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return ctx
}
func (noopTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// pgErr builds a *pgconn.PgError carrying the given SQLSTATE.
func pgErr(code string) *pgconn.PgError {
	return &pgconn.PgError{Code: code, Message: "test: " + code}
}

// fastRetry keeps the backoff negligible so retry tests do not sleep.
func fastRetry(attempts int) TxOption {
	return func(c *txConfig) {
		c.retry.attempts = attempts
		c.retry.baseBackoff = time.Microsecond
		c.retry.maxBackoff = time.Microsecond
	}
}

// txcfg builds a resolved txConfig from the DB default plus the given options,
// as DB.RunInTx does before delegating to runInTx.
func txcfg(opts ...TxOption) txConfig {
	c := txConfig{retry: defaultRetryConfig()}
	for _, o := range opts {
		o(&c)
	}
	return c
}

func newMock(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	m, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(m.Close)
	return m
}

func TestRetryable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"serialization failure", pgErr(sqlStateSerializationFailure), true},
		{"deadlock detected", pgErr(sqlStateDeadlockDetected), true},
		{"wrapped serialization", fmt.Errorf("outer: %w", pgErr(sqlStateSerializationFailure)), true},
		{"unique violation", pgErr("23505"), false},
		{"plain error", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Retryable(tc.err); got != tc.want {
				t.Errorf("Retryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestBackoffCappedExponential(t *testing.T) {
	t.Parallel()
	base, max := 10*time.Millisecond, 100*time.Millisecond
	want := []time.Duration{10, 20, 40, 80, 100, 100}
	for i, w := range want {
		attempt := i + 1
		got := backoff(attempt, base, max)
		if got != w*time.Millisecond {
			t.Errorf("backoff(%d) = %v, want %v", attempt, got, w*time.Millisecond)
		}
	}
	if got := backoff(0, base, max); got != base {
		t.Errorf("backoff(0) = %v, want base %v", got, base)
	}
	// A very large attempt must clamp to max, not overflow.
	if got := backoff(1000, base, max); got != max {
		t.Errorf("backoff(1000) = %v, want max %v", got, max)
	}
}

func TestRunInTxCommitsOnSuccess(t *testing.T) {
	t.Parallel()
	m := newMock(t)
	m.ExpectBeginTx(pgx.TxOptions{})
	m.ExpectExec("UPDATE acme").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	m.ExpectCommit()

	ctx := context.Background()
	err := runInTx(ctx, m, txConfig{retry: defaultRetryConfig()}, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE acme SET n = n + 1")
		return e
	})
	if err != nil {
		t.Fatalf("RunInTx: %v", err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRunInTxRetriesSerializationFailureThenSucceeds(t *testing.T) {
	t.Parallel()
	m := newMock(t)
	// First attempt loses the serialization conflict and rolls back.
	m.ExpectBeginTx(pgx.TxOptions{})
	m.ExpectExec("UPDATE acme").WillReturnError(pgErr(sqlStateSerializationFailure))
	m.ExpectRollback()
	// Second attempt commits.
	m.ExpectBeginTx(pgx.TxOptions{})
	m.ExpectExec("UPDATE acme").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	m.ExpectCommit()

	ctx := context.Background()
	attempts := 0
	err := runInTx(ctx, m, txcfg(fastRetry(defaultTxAttempts)), func(tx pgx.Tx) error {
		attempts++
		_, e := tx.Exec(ctx, "UPDATE acme SET n = n + 1")
		return e
	})
	if err != nil {
		t.Fatalf("RunInTx: %v", err)
	}
	if attempts != 2 {
		t.Errorf("fn ran %d times, want 2", attempts)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRunInTxGivesUpAfterMaxAttempts(t *testing.T) {
	t.Parallel()
	m := newMock(t)
	const attempts = 3
	for i := 0; i < attempts; i++ {
		m.ExpectBeginTx(pgx.TxOptions{})
		m.ExpectExec("UPDATE acme").WillReturnError(pgErr(sqlStateDeadlockDetected))
		m.ExpectRollback()
	}

	ctx := context.Background()
	ran := 0
	err := runInTx(ctx, m, txcfg(fastRetry(attempts)), func(tx pgx.Tx) error {
		ran++
		_, e := tx.Exec(ctx, "UPDATE acme SET n = n + 1")
		return e
	})
	if err == nil {
		t.Fatal("RunInTx should fail after exhausting retries")
	}
	if !Retryable(err) {
		t.Errorf("exhausted error should still unwrap to a retryable PgError: %v", err)
	}
	if ran != attempts {
		t.Errorf("fn ran %d times, want %d", ran, attempts)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRunInTxDoesNotRetryNonRetryable(t *testing.T) {
	t.Parallel()
	m := newMock(t)
	m.ExpectBeginTx(pgx.TxOptions{})
	m.ExpectExec("UPDATE acme").WillReturnError(pgErr("23505")) // unique violation
	m.ExpectRollback()

	ctx := context.Background()
	ran := 0
	err := runInTx(ctx, m, txcfg(fastRetry(defaultTxAttempts)), func(tx pgx.Tx) error {
		ran++
		_, e := tx.Exec(ctx, "UPDATE acme SET n = n + 1")
		return e
	})
	if err == nil {
		t.Fatal("a non-retryable error should surface immediately")
	}
	if ran != 1 {
		t.Errorf("fn ran %d times, want 1 (no retry)", ran)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRunInTxReturnsBeginError(t *testing.T) {
	t.Parallel()
	m := newMock(t)
	m.ExpectBeginTx(pgx.TxOptions{}).WillReturnError(errors.New("pool exhausted"))

	err := runInTx(context.Background(), m, txConfig{retry: defaultRetryConfig()}, func(pgx.Tx) error {
		t.Fatal("fn must not run when BeginTx fails")
		return nil
	})
	if err == nil {
		t.Fatal("BeginTx failure should surface")
	}
}

func TestRunInTxHonorsContextCancellation(t *testing.T) {
	t.Parallel()
	m := newMock(t)
	m.ExpectBeginTx(pgx.TxOptions{})
	m.ExpectExec("UPDATE acme").WillReturnError(pgErr(sqlStateSerializationFailure))
	m.ExpectRollback()

	ctx, cancel := context.WithCancel(context.Background())
	err := runInTx(ctx, m, txcfg(func(c *txConfig) {
		c.retry.attempts = 5
		c.retry.baseBackoff = time.Hour // force the select to wait on ctx
		c.retry.maxBackoff = time.Hour
	}), func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE acme SET n = n + 1")
		cancel() // cancel before the backoff wait so the next wait aborts
		return e
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestRunInTxPassesTxOptions(t *testing.T) {
	t.Parallel()
	m := newMock(t)
	want := pgx.TxOptions{IsoLevel: pgx.Serializable}
	m.ExpectBeginTx(want)
	m.ExpectCommit()

	err := runInTx(context.Background(), m, txConfig{retry: defaultRetryConfig(), txOpts: want}, func(pgx.Tx) error {
		return nil
	})
	if err != nil {
		t.Fatalf("RunInTx: %v", err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations (isolation not passed through?): %v", err)
	}
}

func TestDBMethodsWithoutServer(t *testing.T) {
	t.Parallel()
	// pgxpool.New connects lazily, so a pool to an unreachable address is created
	// without a server; the probe methods then exercise the failure path.
	pool, err := pgxpool.New(context.Background(), "postgres://user:pass@127.0.0.1:1/acme")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	db := &DB{pool: pool, retry: defaultRetryConfig()}
	defer db.Close()

	if db.Pool() != pool {
		t.Error("Pool() did not return the underlying pool")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if db.Healthy(ctx) {
		t.Error("Healthy = true against an unreachable server, want false")
	}
	if err := db.Ping(ctx); err == nil {
		t.Error("Ping should fail against an unreachable server")
	}
}

func TestNewRejectsBadDSN(t *testing.T) {
	t.Parallel()
	_, err := New(context.Background(), "://not a dsn")
	if err == nil {
		t.Fatal("New with an invalid DSN should fail")
	}
}

func TestNewFailsWhenUnreachable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Nothing listens on 127.0.0.1:1; a short connect timeout keeps the test fast.
	_, err := New(ctx,
		"postgres://user:pass@127.0.0.1:1/acme",
		WithConnectTimeout(200*time.Millisecond),
	)
	if err == nil {
		t.Fatal("New against an unreachable server should fail")
	}
}
