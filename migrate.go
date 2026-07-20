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

// Migrate and MigrateWithTable run a directory of plain SQL migrations against
// a PostgreSQL database using github.com/golang-migrate/migrate/v4 (the
// "postgres" database driver and the "file" source driver). They are a thin,
// opinionated seam over golang-migrate for the common case: apply every
// pending "up" migration and return. Reaching for golang-migrate directly is
// still fine for anything more advanced (Down, Steps, Version, a different
// source such as an embedded filesystem, and so on) -- these two functions
// exist only to remove the small amount of boilerplate (opening a migrator,
// treating ErrNoChange as success, closing it) most services repeat.
//
// The migrations directory must contain paired "*.up.sql" / "*.down.sql"
// files named "<version>_<description>.up.sql" / ".down.sql", golang-migrate's
// standard layout. Migrate and MigrateWithTable open their own database
// connection for the migration run and close it before returning; they do not
// use a DB built by New, so they can run before a pool exists (for example, in
// a startup script or an init container).

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // registers the "postgres" database driver
	_ "github.com/golang-migrate/migrate/v4/source/file"       // registers the "file" source driver
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
	dsn, err := withMigrationsTable(dsn, table)
	if err != nil {
		return err
	}

	m, err := migrate.New("file://"+migrationsDir, dsn)
	if err != nil {
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
