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

// Migrate, MigrateWithTable, MigrateFS and MigrateFSWithTable run a set of
// plain SQL migrations against a PostgreSQL database using
// github.com/golang-migrate/migrate/v4 (the "postgres" database driver and
// the "iofs" source). They are a thin, opinionated seam over golang-migrate
// for the common case: apply every pending "up" migration and return.
// Reaching for golang-migrate directly is still fine for anything more
// advanced (Down, Steps, Version, and so on) -- these functions exist only to
// remove the small amount of boilerplate (opening a migrator, treating
// ErrNoChange as success, closing it) most services repeat.
//
// Migrate and MigrateWithTable read a directory on disk. MigrateFS and
// MigrateFSWithTable read a directory inside any fs.FS, so migrations
// embedded in the binary with embed.FS run without being unpacked first. The
// directory forms are wrappers over the fs.FS forms through os.DirFS, which
// is also how golang-migrate's own "file" source reads a directory.
//
// The migrations directory must contain paired "*.up.sql" / "*.down.sql"
// files named "<version>_<description>.up.sql" / ".down.sql", golang-migrate's
// standard layout. Every function here opens its own database connection for
// the migration run and closes it before returning; none uses a DB built by
// New, so they can run before a pool exists (for example, in a startup script
// or an init container).
//
// Migrations require a direct (session) PostgreSQL connection and MUST NOT run
// through a transaction-pooled PgBouncer. golang-migrate relies on session
// advisory locks, SELECT CURRENT_SCHEMA(), and prepared statements, all of
// which transaction pooling breaks. Callers that route normal traffic through
// PgBouncer should point the migration dsn at the primary (or a session-mode
// port) instead.

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // registers the "postgres" database driver
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrate runs every pending "up" migration in migrationsDir against dsn and
// returns once the schema is current. dsn is a standard PostgreSQL connection
// string, for example "postgres://user:pass@localhost:5432/acme". ErrNoChange
// (there was nothing to migrate) is treated as success, not an error.
func Migrate(dsn, migrationsDir string) error {
	return MigrateWithTable(dsn, migrationsDir, "")
}

// MigrateWithTable is Migrate, but records applied versions in table instead
// of golang-migrate's default "schema_migrations". Use it when two
// independent sets of migrations run against the same database -- for
// example, a service's own schema alongside an embedded library's -- so each
// set tracks its own versions and neither skips the other's migrations. An
// empty table falls back to the default.
func MigrateWithTable(dsn, migrationsDir, table string) error {
	if migrationsDir == "" {
		// golang-migrate's file source reads an empty path as the working
		// directory; os.DirFS rejects an empty root, so keep the old meaning.
		migrationsDir = "."
	}
	return MigrateFSWithTable(dsn, os.DirFS(migrationsDir), ".", table)
}

// MigrateFS is Migrate, but reads the migrations from dir inside fsys instead
// of from a directory on disk. fsys can be any fs.FS -- most usefully an
// embed.FS, so a service ships its migrations inside its binary:
//
//	//go:embed migrations
//	var migrations embed.FS
//
//	err := postgres.MigrateFS(dsn, migrations, "migrations")
//
// dir is a slash-separated path within fsys, as fs.ValidPath defines it; use
// "." when the migrations sit at the root of fsys.
func MigrateFS(dsn string, fsys fs.FS, dir string) error {
	return MigrateFSWithTable(dsn, fsys, dir, "")
}

// MigrateFSWithTable is MigrateFS, but records applied versions in table
// instead of golang-migrate's default "schema_migrations", exactly as
// MigrateWithTable does for a directory on disk. An empty table falls back to
// the default.
func MigrateFSWithTable(dsn string, fsys fs.FS, dir, table string) error {
	dsn, err := withMigrationsTable(dsn, table)
	if err != nil {
		return err
	}

	src, err := iofs.New(fsys, dir)
	if err != nil {
		return fmt.Errorf("postgres: open migrator: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		_ = src.Close()
		return fmt.Errorf("postgres: open migrator: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("postgres: migrate up: %w", err)
	}
	return nil
}

// withMigrationsTable returns dsn with its "x-migrations-table" query
// parameter set to table, which golang-migrate's postgres driver reads to
// pick the version-tracking table. An empty table returns dsn unchanged.
func withMigrationsTable(dsn, table string) (string, error) {
	if table == "" {
		return dsn, nil
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("postgres: parse dsn: %w", err)
	}
	q := u.Query()
	q.Set("x-migrations-table", table)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
