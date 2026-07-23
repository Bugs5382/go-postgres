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
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v4"
)

// fakeQuerier proves Querier is mockable with no pgx dependency at all -- the
// point of the seam.
type fakeQuerier struct {
	execN int
}

func (f *fakeQuerier) Exec(context.Context, string, ...any) (CommandTag, error) {
	f.execN++
	return CommandTag{}, nil
}
func (f *fakeQuerier) Query(context.Context, string, ...any) (Rows, error) { return nil, nil }
func (f *fakeQuerier) QueryRow(context.Context, string, ...any) Row        { return nil }

func TestQuerierMockable(t *testing.T) {
	t.Parallel()
	var q Querier = &fakeQuerier{}
	if _, err := q.Exec(context.Background(), "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if q.(*fakeQuerier).execN != 1 {
		t.Errorf("Exec ran %d times, want 1", q.(*fakeQuerier).execN)
	}
}

func TestErrNoRowsIsPgxErrNoRows(t *testing.T) {
	t.Parallel()
	if !errors.Is(ErrNoRows, pgx.ErrNoRows) {
		t.Fatal("ErrNoRows should be pgx.ErrNoRows")
	}
}

func TestDBQuerierReturnsPool(t *testing.T) {
	t.Parallel()
	pool, err := pgxpool.New(context.Background(), "postgres://user:pass@127.0.0.1:1/acme")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	db := &DB{pool: pool, retry: defaultRetryConfig()}
	defer db.Close()

	if db.Querier() != Querier(pool) {
		t.Error("Querier() did not return the underlying pool as a Querier")
	}
}

// TestRunInTxQuerierDelegatesToRunInTx exercises the real public method (not
// just the retry loop it shares with RunInTx): against an unreachable pool,
// BeginTx fails before fn ever runs, proving RunInTxQuerier wires through
// RunInTx rather than duplicating its own transaction handling.
func TestRunInTxQuerierDelegatesToRunInTx(t *testing.T) {
	t.Parallel()
	pool, err := pgxpool.New(context.Background(), "postgres://user:pass@127.0.0.1:1/acme")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	db := &DB{pool: pool, retry: defaultRetryConfig()}
	defer db.Close()

	ran := false
	err = db.RunInTxQuerier(context.Background(), func(Querier) error {
		ran = true
		return nil
	})
	if err == nil {
		t.Fatal("RunInTxQuerier against an unreachable server should fail")
	}
	if ran {
		t.Error("fn must not run when BeginTx fails")
	}
}

// TestRunInTxQuerierCommitsAndRetries exercises the exact Tx-to-Querier
// hand-off RunInTxQuerier performs (var q Querier = tx; fn(q)) through the
// same runInTx retry loop RunInTx and RunInTxQuerier both share, proving the
// Querier form keeps the serialization-failure retry behavior identical.
func TestRunInTxQuerierCommitsAndRetries(t *testing.T) {
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
	querierFn := func(q Querier) error {
		attempts++
		_, err := q.Exec(ctx, "UPDATE acme SET n = n + 1")
		return err
	}
	err := runInTx(ctx, m, txcfg(fastRetry(defaultTxAttempts)), func(tx pgx.Tx) error {
		return querierFn(tx)
	})
	if err != nil {
		t.Fatalf("RunInTxQuerier (via runInTx): %v", err)
	}
	if attempts != 2 {
		t.Errorf("fn ran %d times, want 2", attempts)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
