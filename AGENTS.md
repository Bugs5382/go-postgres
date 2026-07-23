# AGENTS.md - go-postgres

Guide for AI agents working in this repository. Pair with `CLAUDE.md` (the working agreement and
hook-enforced rules). Keep this file current when the build, layout, or public API changes.

## What this is

A small, dependency-light PostgreSQL helper for Go. It wraps
`github.com/jackc/pgx/v5` (`pgxpool`) so a service gets a correctly configured
connection pool -- sensible sizes, a connect timeout, a health-check period, and
a readiness Ping -- from a single `New` call, plus a serialization-failure retry
helper for transactions, instead of re-deriving that configuration in every codebase.

## Using go-postgres

The public surface is small and additive; keep it stable:

- `New(ctx, dsn, ...Option) (*DB, error)` -- parses the DSN, builds a `pgxpool.Pool` with resilient
  defaults, and Pings to verify readiness before returning.
- `(*DB).Pool() *pgxpool.Pool` -- the underlying pool for the full pgx API. `(*DB).Ping(ctx) error`
  and `(*DB).Healthy(ctx) bool` for probes. `(*DB).Close()` releases the pool.
- `(*DB).RunInTx(ctx, fn, ...TxOption) error` -- runs `fn` inside a transaction and retries on
  serialization_failure (SQLSTATE 40001) and deadlock_detected (40P01) with capped exponential
  backoff. `Retryable(err) bool` reports whether an error is one of those codes.
- `Tx`, `Rows`, `Row`, `CommandTag` (type aliases) and `ErrNoRows` (var) re-export the matching pgx
  and pgconn types, so a caller can name them without importing `github.com/jackc/pgx/v5` directly.
  `Querier` is the minimal `Exec`/`Query`/`QueryRow` interface both a pool and a `Tx` satisfy without
  any adapter. `(*DB).Querier() Querier` hands out the pool as a `Querier`. `(*DB).RunInTxQuerier(ctx,
  fn func(Querier) error, ...TxOption) error` is `RunInTx`, but hands `fn` the transaction as a
  `Querier` instead of a `Tx` -- it delegates to `RunInTx`, sharing its retry loop and `TxOption`s
  exactly, so a store built against `Querier` runs unchanged via `Querier()` or `RunInTxQuerier`.
- Pool options: `WithMaxConns`, `WithMinConns`, `WithMaxConnLifetime`, `WithMaxConnIdleTime`,
  `WithHealthCheckPeriod`, `WithConnectTimeout`, `WithTxRetry(attempts, base, max)`, and
  `WithTracer` (a pgx `QueryTracer`, so the core carries no OpenTelemetry dependency).
- Tx options: `WithTxOptions`, `WithIsolation`, `WithAttempts`, `WithBackoff`. Non-positive tuning
  values keep the default.
- `Migrate(dsn, migrationsDir) error` / `MigrateWithTable(dsn, migrationsDir, table) error` -- run a
  directory of paired `*.up.sql`/`*.down.sql` files with `golang-migrate`; `ErrNoChange` is success.
  `MigrateWithTable` sets a custom `x-migrations-table` so two migration sets can share a database.
  Neither needs a `DB`/pool, so either can run before `New`.
- The `otel` subpackage adds OpenTelemetry tracing via `WithTracing` (built on `exaring/otelpgx`),
  keeping the OpenTelemetry dependency out of the core, plus `InstrumentMigrate`, which wraps a
  migration run with a span, a duration histogram, a count (via `go-otel`), and trace-correlated
  structured logs (via `go-log`).

## Layout

- `postgres.go` - `DB`, `New`, the `Pool`/`Querier`/`Ping`/`Healthy`/`Close` methods, `RunInTx`/
  `RunInTxQuerier`, and the serialization-failure retry loop.
- `querier.go` - the `Tx`/`Rows`/`Row`/`CommandTag` aliases, `ErrNoRows`, and the `Querier` interface.
- `options.go` - the `Option`/`TxOption` types, all `With*` options, the resilient defaults, and the
  mapping onto the `pgxpool.Config`.
- `migrate.go` - `Migrate`, `MigrateWithTable`, and the `golang-migrate` wiring behind them.
- `otel/` - the optional OpenTelemetry adapter (`WithTracing`, `InstrumentMigrate`), a separate
  import path.
- `doc.go` - package doc.
- `*_test.go` - unit tests using `pashagolub/pgxmock` (no live Postgres); `integration_test.go` is
  behind `//go:build integration` and reads `POSTGRES_DSN`.

## Build, test, lint

- Build: `task build`
- Test: `task test` (no external service; tests use pgxmock) / `task test-integration` (needs a server)
- Full gate: `task ci` (build + vet + race tests + gofmt + golangci-lint + yamllint)
- License headers: `task license` (verify) / `task license:fix` (inject)

## Conventions and gotchas

- See `CLAUDE.md` for the branch/commit/PR rules; they are enforced by the git hooks in
  `.claude/hooks` (run `bash .claude/hooks/install.sh` once per clone).
- Keep the `New`/`DB` surface stable; add capabilities additively.
- The core must stay free of any OpenTelemetry dependency -- tracing goes through the pgx
  `QueryTracer` seam (`WithTracer`) or the `otel` subpackage.
- `RunInTx` only retries on 40001/40P01; any other error (including a caller error from `fn`) fails
  fast without a retry. Keep that discrimination in `Retryable`.
- `RunInTxQuerier` exists *alongside* `RunInTx`, not in place of it: Go's function types are
  invariant, so retyping `RunInTx`'s callback from `Tx` to `Querier` would break every existing
  caller that names the parameter type. Add new capabilities as new methods for the same reason;
  don't retype an existing one.
