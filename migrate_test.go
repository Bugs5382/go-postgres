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
	"strings"
	"testing"
)

func TestWithMigrationsTableEmptyLeavesDSNUnchanged(t *testing.T) {
	t.Parallel()
	got, err := withMigrationsTable(testDSN, "")
	if err != nil {
		t.Fatalf("withMigrationsTable() error = %v", err)
	}
	if got != testDSN {
		t.Errorf("withMigrationsTable() = %q, want %q", got, testDSN)
	}
}

func TestWithMigrationsTableSetsQueryParam(t *testing.T) {
	t.Parallel()
	got, err := withMigrationsTable(testDSN, "app_migrations")
	if err != nil {
		t.Fatalf("withMigrationsTable() error = %v", err)
	}
	if !strings.Contains(got, "x-migrations-table=app_migrations") {
		t.Errorf("withMigrationsTable() = %q, want it to carry x-migrations-table=app_migrations", got)
	}
}

func TestWithMigrationsTablePreservesExistingQuery(t *testing.T) {
	t.Parallel()
	dsn := testDSN + "?sslmode=disable"
	got, err := withMigrationsTable(dsn, "app_migrations")
	if err != nil {
		t.Fatalf("withMigrationsTable() error = %v", err)
	}
	if !strings.Contains(got, "sslmode=disable") || !strings.Contains(got, "x-migrations-table=app_migrations") {
		t.Errorf("withMigrationsTable() = %q, want both sslmode and x-migrations-table", got)
	}
}

func TestWithMigrationsTableRejectsUnparseableDSN(t *testing.T) {
	t.Parallel()
	if _, err := withMigrationsTable("postgres://%zz", "app_migrations"); err == nil {
		t.Fatal("withMigrationsTable() error = nil, want an error for an unparseable dsn")
	}
}

// Migrate opens the source directory before it ever dials the database, so a
// missing migrations directory fails fast with no network involved -- this
// exercises that error path without a live Postgres server.
func TestMigrateMissingDirectory(t *testing.T) {
	t.Parallel()
	if err := Migrate(testDSN, "/no/such/migrations/dir"); err == nil {
		t.Fatal("Migrate() error = nil, want an error for a missing migrations directory")
	}
}

func TestMigrateWithTableMissingDirectory(t *testing.T) {
	t.Parallel()
	if err := MigrateWithTable(testDSN, "/no/such/migrations/dir", "app_migrations"); err == nil {
		t.Fatal("MigrateWithTable() error = nil, want an error for a missing migrations directory")
	}
}
