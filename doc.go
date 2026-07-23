// Package postgres wraps github.com/jackc/pgx/v5 (pgxpool) with resilient
// defaults so a service gets a correctly configured PostgreSQL connection pool
// from a single New call: sensible pool sizes, a connect timeout, a health-check
// period, and a startup Ping that verifies readiness. Functional options tune the
// pool, timeouts, and an optional pgx QueryTracer. RunInTx runs a function inside
// a transaction and retries automatically on serialization failures (SQLSTATE
// 40001) and deadlocks (40P01) with capped exponential backoff. The returned DB
// exposes the underlying *pgxpool.Pool for the full pgx API.
//
// Tx, Rows, Row, CommandTag, and ErrNoRows re-export the matching pgx (and
// pgconn) types, and Querier is the minimal Exec/Query/QueryRow interface both
// a pool and a transaction satisfy. Building a store or repository against
// Querier -- reached through DB.Querier (outside a transaction) or
// RunInTxQuerier (inside one) -- lets that code depend on this package alone,
// with no direct import of github.com/jackc/pgx/v5.
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
