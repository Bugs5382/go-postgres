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
	"io/fs"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

// unreachableDSN points at a port nothing listens on, so a migration run that
// gets past its source fails on the dial without needing a live server.
const unreachableDSN = "postgres://acme@127.0.0.1:1/acme?sslmode=disable&connect_timeout=1"

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

func TestMigrateFSMissingDirectory(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{}
	if err := MigrateFS(testDSN, fsys, "migrations"); err == nil {
		t.Fatal("MigrateFS() error = nil, want an error for a directory missing from the fs.FS")
	}
}

func TestMigrateFSWithTableMissingDirectory(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{}
	if err := MigrateFSWithTable(testDSN, fsys, "migrations", "app_migrations"); err == nil {
		t.Fatal("MigrateFSWithTable() error = nil, want an error for a directory missing from the fs.FS")
	}
}

// recordingFS is an fs.FS that remembers every name it was asked to open, so a
// test can prove the migration source was read from it rather than from disk.
type recordingFS struct {
	fs.FS
	mu     sync.Mutex
	opened []string
}

func (r *recordingFS) Open(name string) (fs.File, error) {
	r.mu.Lock()
	r.opened = append(r.opened, name)
	r.mu.Unlock()
	return r.FS.Open(name)
}

func TestMigrateFSReadsMigrationsFromFS(t *testing.T) {
	t.Parallel()
	fsys := &recordingFS{FS: fstest.MapFS{
		"db/migrations/1_widgets.up.sql":   {Data: []byte("CREATE TABLE widgets (id serial PRIMARY KEY);")},
		"db/migrations/1_widgets.down.sql": {Data: []byte("DROP TABLE widgets;")},
	}}

	// Nothing listens on the dsn, so the run must fail -- but only after the
	// source was opened from fsys.
	if err := MigrateFS(unreachableDSN, fsys, "db/migrations"); err == nil {
		t.Fatal("MigrateFS() error = nil, want a dial error for an unreachable server")
	}

	fsys.mu.Lock()
	defer fsys.mu.Unlock()
	found := false
	for _, name := range fsys.opened {
		if name == "db/migrations" {
			found = true
		}
	}
	if !found {
		t.Errorf("MigrateFS() opened %v from the fs.FS, want it to read %q", fsys.opened, "db/migrations")
	}
}

// An empty migrationsDir has always meant the working directory (golang-migrate's
// file source resolves an empty path that way); routing Migrate through an
// fs.FS must keep that, rather than failing on an empty os.DirFS root.
func TestMigrateEmptyDirectoryMeansWorkingDirectory(t *testing.T) {
	t.Parallel()
	err := Migrate(unreachableDSN, "")
	if err == nil {
		t.Fatal("Migrate() error = nil, want a dial error for an unreachable server")
	}
	if strings.Contains(err.Error(), "open migrator: open") || strings.Contains(err.Error(), "empty root") {
		t.Errorf("Migrate(dsn, \"\") error = %v, want the source to open and the dial to fail", err)
	}
}
