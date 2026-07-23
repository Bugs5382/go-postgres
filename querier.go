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

// This file re-exports the small slice of github.com/jackc/pgx/v5 (and
// pgconn) types a caller needs to build a query seam -- Tx, Rows, Row,
// CommandTag, ErrNoRows, and the Querier interface they satisfy -- as type
// aliases and a var. An alias is the same type as the pgx original, so values
// flow between postgres.* and pgx.* with no conversion; the point is only
// that a caller can name the type as postgres.Tx instead of importing
// github.com/jackc/pgx/v5 directly, which matters for code that wants to
// depend on this package alone.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type (
	// Tx is a pgx transaction, the type RunInTx's callback receives. It is an
	// alias for pgx.Tx, re-exported so a caller can name it without importing
	// github.com/jackc/pgx/v5 directly.
	Tx = pgx.Tx

	// Rows is a pgx result set, returned by Querier.Query. It is an alias for
	// pgx.Rows, re-exported so a caller can name it without importing
	// github.com/jackc/pgx/v5 directly.
	Rows = pgx.Rows

	// Row is a single-row pgx result, returned by Querier.QueryRow. It is an
	// alias for pgx.Row, re-exported so a caller can name it without
	// importing github.com/jackc/pgx/v5 directly.
	Row = pgx.Row

	// CommandTag is the tag Querier.Exec returns on success. It is an alias
	// for pgconn.CommandTag, re-exported so a caller can name it without
	// importing github.com/jackc/pgx/v5/pgconn directly.
	CommandTag = pgconn.CommandTag
)

// ErrNoRows is returned by QueryRow's Row.Scan when a query matched no rows.
// It is pgx.ErrNoRows, re-exported so a caller can check it with errors.Is
// without importing github.com/jackc/pgx/v5 directly.
var ErrNoRows = pgx.ErrNoRows

// Querier is the minimal query surface a store or repository should depend
// on instead of *pgxpool.Pool or Tx directly: Exec, Query, and QueryRow. Both
// a pool (DB.Querier) and a transaction (Tx, the argument RunInTx's callback
// receives) satisfy it, so the same code runs standalone or inside a
// transaction, and a unit test can supply a hand-written fake with no pgx
// dependency at all.
type Querier interface {
	// Exec runs sql (INSERT/UPDATE/DELETE/DDL, or any statement that returns
	// no rows) and reports the resulting command tag.
	Exec(ctx context.Context, sql string, args ...any) (CommandTag, error)

	// Query runs sql and returns the resulting rows. The caller must Close
	// (or fully read to completion) the returned Rows.
	Query(ctx context.Context, sql string, args ...any) (Rows, error)

	// QueryRow runs sql and returns a Row wrapping at most one result row.
	// Any error from the query itself surfaces from the returned Row's Scan,
	// as ErrNoRows when there was no matching row.
	QueryRow(ctx context.Context, sql string, args ...any) Row
}

// *pgxpool.Pool and Tx already satisfy Querier -- no adapter needed. These
// compile-time assertions catch a signature drift in either type early.
var (
	_ Querier = (*pgxpool.Pool)(nil)
	_ Querier = Tx(nil)
)
