# go-postgres 🐘

> Resilient PostgreSQL wiring for Go in one call — a sensible `pgxpool`, connect timeout, and health check by default, plus a transaction helper that retries serialization failures automatically.

`go-postgres` wraps [`jackc/pgx/v5`](https://github.com/jackc/pgx) (`pgxpool`) so a service stops re-deriving the same pool sizes, timeouts, and retry policy in every codebase. `New` returns a pool that has already Pinged the server, so a successful call means you are ready to serve. `RunInTx` handles the retry loop that serializable workloads need.

## 📦 Install

```bash
go get github.com/Bugs5382/go-postgres
```

## 🚀 Usage

`New` parses the DSN, applies resilient defaults, and verifies readiness with a
Ping. `Pool()` hands you the full pgx command API.

```go
db, err := postgres.New(ctx, "postgres://user:pass@localhost:5432/acme")
if err != nil {
	log.Fatal(err)
}
defer db.Close()

var now time.Time
db.Pool().QueryRow(ctx, "SELECT now()").Scan(&now)
```

## 🔁 Serialization-failure retries

`RunInTx` runs your function inside a transaction and retries the whole thing on
`serialization_failure` (SQLSTATE `40001`) and `deadlock_detected` (`40P01`) with
capped exponential backoff — the retry loop every `SERIALIZABLE` workload needs.
Make the function idempotent; it may run more than once.

```go
err := db.RunInTx(ctx, func(tx pgx.Tx) error {
	var balance int
	if err := tx.QueryRow(ctx, "SELECT balance FROM accounts WHERE id = $1", id).Scan(&balance); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "UPDATE accounts SET balance = $1 WHERE id = $2", balance-100, id)
	return err
}, postgres.WithIsolation(pgx.Serializable))
```

Any other error — including one your function returns — aborts immediately
without a retry. `Retryable(err)` reports whether an error is one of the two
transient codes.

## 🎛 Options

Every knob is a functional option, and every one has a resilient default.

```go
db, _ := postgres.New(ctx, dsn,
	postgres.WithMaxConns(50),
	postgres.WithMinConns(2),
	postgres.WithMaxConnLifetime(time.Hour),
	postgres.WithMaxConnIdleTime(30*time.Minute),
	postgres.WithHealthCheckPeriod(time.Minute),
	postgres.WithConnectTimeout(5*time.Second),
	postgres.WithTxRetry(10, 5*time.Millisecond, time.Second),
)
```

Per-transaction options override the defaults for a single call:
`WithTxOptions`, `WithIsolation`, `WithAttempts`, and `WithBackoff`.

## 💓 Health

`Healthy` is a cheap Ping — wire it straight into a readiness or liveness probe.

```go
if !db.Healthy(ctx) {
	// fail the probe
}
```

## 🧱 Migrations

`Migrate` and `MigrateWithTable` run a directory of plain SQL migrations with
[`golang-migrate`](https://github.com/golang-migrate/migrate) -- no pool
required, so they can run before `New` (for example, at startup or from an
init container). The directory holds paired `*.up.sql`/`*.down.sql` files;
`ErrNoChange` counts as success.

```go
if err := postgres.Migrate(dsn, "./migrations"); err != nil {
	log.Fatal(err)
}
```

`MigrateWithTable` records applied versions in a table other than the default
`schema_migrations`, so a service's own migrations coexist in the same
database as an embedded library's migrations, each tracking its own versions
independently:

```go
err := postgres.MigrateWithTable(dsn, "./migrations", "app_migrations")
```

## 📊 OpenTelemetry

The optional [`otel`](./otel) subpackage installs an
[`otelpgx`](https://github.com/exaring/otelpgx) query tracer through the
`WithTracer` seam, so the core carries no OpenTelemetry dependency. Configure the
global `TracerProvider` (for example with
[`go-otel`](https://github.com/Bugs5382/go-otel)) first.

```go
import otelpg "github.com/Bugs5382/go-postgres/otel"

db, _ := postgres.New(ctx, dsn, otelpg.WithTracing()) // span per query
```

The same subpackage's `InstrumentMigrate` wraps a migration run with a span, a
duration histogram, a count, and structured, trace-correlated logs (via
[`go-log`](https://github.com/Bugs5382/go-log)):

```go
err := otelpg.InstrumentMigrate(ctx, "app", func() error {
	return postgres.Migrate(dsn, "./migrations")
})
```

## 🛠 Develop

```bash
task build    # go build ./...
task test     # go test ./...
task ci       # build + vet + race tests + linters
task license  # verify MIT headers (golic)
```

Integration tests run against a live server and are excluded from the default build:

```bash
POSTGRES_DSN=postgres://user:pass@localhost:5432/acme task test-integration
```

## ⚖️ License

MIT © 2026 Shane
