package postgres_test

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
	"log"
	"time"

	postgres "github.com/Bugs5382/go-postgres"
	"github.com/jackc/pgx/v5"
)

// Connect to a server with the resilient defaults, then query through the
// underlying pgx pool.
func ExampleNew() {
	ctx := context.Background()

	db, err := postgres.New(ctx, "postgres://user:pass@localhost:5432/acme")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var now time.Time
	if err := db.Pool().QueryRow(ctx, "SELECT now()").Scan(&now); err != nil {
		log.Fatal(err)
	}
}

// Tune the pool and the transaction retry policy through functional options.
func ExampleNew_options() {
	ctx := context.Background()

	db, err := postgres.New(ctx,
		"postgres://user:pass@localhost:5432/acme",
		postgres.WithMaxConns(50),
		postgres.WithConnectTimeout(5*time.Second),
		postgres.WithHealthCheckPeriod(time.Minute),
		postgres.WithTxRetry(10, 5*time.Millisecond, time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
}

// RunInTx retries automatically on serialization failures, which are expected
// under Serializable isolation. fn must be safe to run more than once.
func ExampleDB_RunInTx() {
	ctx := context.Background()

	db, err := postgres.New(ctx, "postgres://user:pass@localhost:5432/acme")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = db.RunInTx(ctx, func(tx pgx.Tx) error {
		var balance int
		if err := tx.QueryRow(ctx, "SELECT balance FROM accounts WHERE id = $1", 1).Scan(&balance); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE accounts SET balance = $1 WHERE id = $2", balance-100, 1)
		return err
	}, postgres.WithIsolation(pgx.Serializable))
	if err != nil {
		log.Fatal(err)
	}
}

// Build a store against Querier instead of *pgxpool.Pool, and reach it either
// through DB.Querier (outside a transaction) or RunInTxQuerier (inside one).
// Neither the store nor this example needs to import github.com/jackc/pgx/v5.
func ExampleDB_Querier() {
	ctx := context.Background()

	db, err := postgres.New(ctx, "postgres://user:pass@localhost:5432/acme")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if _, err := accountBalance(ctx, db.Querier(), 1); err != nil {
		log.Fatal(err)
	}
}

// accountBalance is a store method built against postgres.Querier, so it runs
// unchanged whether q is a plain pool (DB.Querier) or a transaction
// (RunInTxQuerier's argument).
func accountBalance(ctx context.Context, q postgres.Querier, id int) (int, error) {
	var balance int
	err := q.QueryRow(ctx, "SELECT balance FROM accounts WHERE id = $1", id).Scan(&balance)
	if errors.Is(err, postgres.ErrNoRows) {
		return 0, fmt.Errorf("account %d not found", id)
	}
	return balance, err
}

// RunInTxQuerier is RunInTx, but hands the callback a Querier instead of a
// pgx.Tx, so the same fn can share a store's Querier-based methods with code
// that also runs outside a transaction (see ExampleDB_Querier).
func ExampleDB_RunInTxQuerier() {
	ctx := context.Background()

	db, err := postgres.New(ctx, "postgres://user:pass@localhost:5432/acme")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = db.RunInTxQuerier(ctx, func(q postgres.Querier) error {
		balance, err := accountBalance(ctx, q, 1)
		if err != nil {
			return err
		}
		_, err = q.Exec(ctx, "UPDATE accounts SET balance = $1 WHERE id = $2", balance-100, 1)
		return err
	}, postgres.WithIsolation(pgx.Serializable))
	if err != nil {
		log.Fatal(err)
	}
}

// Migrate applies every pending "up" migration in a directory of paired
// *.up.sql/*.down.sql files before a service starts serving traffic.
func ExampleMigrate() {
	if err := postgres.Migrate("postgres://user:pass@localhost:5432/acme", "./migrations"); err != nil {
		log.Fatal(err)
	}
}

// MigrateWithTable records applied versions in a table other than the default
// "schema_migrations", so a service's own migrations coexist in the same
// database as an embedded library's, each tracking its own versions.
func ExampleMigrateWithTable() {
	err := postgres.MigrateWithTable("postgres://user:pass@localhost:5432/acme", "./migrations", "app_migrations")
	if err != nil {
		log.Fatal(err)
	}
}
