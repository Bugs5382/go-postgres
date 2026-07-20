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
	"path/filepath"
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

// TestIntegrationMigrate exercises Migrate and MigrateWithTable against a real
// PostgreSQL: two independent migration sets, applied with each function, land
// in the same database without colliding -- Migrate's under the default
// "schema_migrations" table, MigrateWithTable's under its own. Run it the same
// way as TestIntegrationRoundTrip.
func TestIntegrationMigrate(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set POSTGRES_DSN to run the integration test")
	}
	ctx := context.Background()

	widgetsDir := t.TempDir()
	writeMigration(t, widgetsDir, "1_widgets", "CREATE TABLE widgets (id serial PRIMARY KEY);", "DROP TABLE widgets;")

	gadgetsDir := t.TempDir()
	writeMigration(t, gadgetsDir, "1_gadgets", "CREATE TABLE gadgets (id serial PRIMARY KEY);", "DROP TABLE gadgets;")

	if err := Migrate(dsn, widgetsDir); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := MigrateWithTable(dsn, gadgetsDir, "gadgets_migrations"); err != nil {
		t.Fatalf("MigrateWithTable: %v", err)
	}

	db, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	for _, table := range []string{"widgets", "schema_migrations", "gadgets", "gadgets_migrations"} {
		var exists bool
		err := db.Pool().QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)", table,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("check %s exists: %v", table, err)
		}
		if !exists {
			t.Errorf("table %q was not created", table)
		}
	}
}

// writeMigration writes a paired up/down SQL migration named version into dir,
// golang-migrate's expected "*.up.sql"/"*.down.sql" layout.
func writeMigration(t *testing.T, dir, version, up, down string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, version+".up.sql"), []byte(up), 0o600); err != nil {
		t.Fatalf("write up migration: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, version+".down.sql"), []byte(down), 0o600); err != nil {
		t.Fatalf("write down migration: %v", err)
	}
}
