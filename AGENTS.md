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
- Pool options: `WithMaxConns`, `WithMinConns`, `WithMaxConnLifetime`, `WithMaxConnIdleTime`,
  `WithHealthCheckPeriod`, `WithConnectTimeout`, `WithTxRetry(attempts, base, max)`, and
  `WithTracer` (a pgx `QueryTracer`, so the core carries no OpenTelemetry dependency).
- Tx options: `WithTxOptions`, `WithIsolation`, `WithAttempts`, `WithBackoff`. Non-positive tuning
  values keep the default.
- The `otel` subpackage adds OpenTelemetry tracing via `WithTracing` (built on `exaring/otelpgx`),
  keeping the OpenTelemetry dependency out of the core.

## Layout

- `postgres.go` - `DB`, `New`, the `Pool`/`Ping`/`Healthy`/`Close` methods, `RunInTx`, and the
  serialization-failure retry loop.
- `options.go` - the `Option`/`TxOption` types, all `With*` options, the resilient defaults, and the
  mapping onto the `pgxpool.Config`.
- `otel/` - the optional OpenTelemetry adapter (`WithTracing`), a separate import path.
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
