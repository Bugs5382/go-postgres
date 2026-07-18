//go:build integration

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
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// TestIntegrationRoundTrip exercises the client against a real PostgreSQL. Run it
// with a live server:
//
//	POSTGRES_DSN=postgres://user:pass@localhost:5432/acme go test -tags=integration ./...
//
// It is excluded from the default build so CI stays green without a database.
func TestIntegrationRoundTrip(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set POSTGRES_DSN to run the integration test")
	}
	ctx := context.Background()

	db, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	if !db.Healthy(ctx) {
		t.Fatal("server not healthy")
	}

	var got int
	err = db.RunInTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT 1").Scan(&got)
	}, WithIsolation(pgx.Serializable))
	if err != nil {
		t.Fatalf("RunInTx: %v", err)
	}
	if got != 1 {
		t.Fatalf("SELECT 1 = %d, want 1", got)
	}
}
