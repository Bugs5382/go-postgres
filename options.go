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
	"runtime"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Default configuration values. They mirror pgx's own well-chosen production
// defaults but are set explicitly so callers can see, and rely on, the resilient
// baseline regardless of upstream changes.
const (
	defaultMinConns          = int32(0)
	defaultMaxConnLifetime   = time.Hour
	defaultMaxConnIdleTime   = 30 * time.Minute
	defaultHealthCheckPeriod = time.Minute
	defaultConnectTimeout    = 5 * time.Second

	defaultTxAttempts    = 10
	defaultTxBaseBackoff = 5 * time.Millisecond
	defaultTxMaxBackoff  = time.Second
)

// maxDefaultPoolConns caps the auto-derived pool ceiling so a very high core
// count does not open an unreasonable number of connections (and keeps the
// GOMAXPROCS conversion safely within int32).
const maxDefaultPoolConns = 100

// defaultMaxConns returns the baseline pool ceiling: pgx uses the greater of 4
// and GOMAXPROCS, so a single-CPU runner still gets a usable pool, clamped to a
// sane maximum.
func defaultMaxConns() int32 {
	n := runtime.GOMAXPROCS(0)
	if n <= 4 {
		return 4
	}
	if n > maxDefaultPoolConns {
		return maxDefaultPoolConns
	}
	return int32(n) //nolint:gosec // n is bounded to (4, maxDefaultPoolConns] above
}

// retryConfig is the transaction retry policy RunInTx applies. A DB carries a
// default policy (from WithTxRetry); a per-call TxOption can override it.
type retryConfig struct {
	attempts    int
	baseBackoff time.Duration
	maxBackoff  time.Duration
}

// defaultRetryConfig returns the baseline transaction retry policy.
func defaultRetryConfig() retryConfig {
	return retryConfig{
		attempts:    defaultTxAttempts,
		baseBackoff: defaultTxBaseBackoff,
		maxBackoff:  defaultTxMaxBackoff,
	}
}

// config is the resolved, internal configuration an Option mutates. It is never
// exposed; New turns it into a concrete pgxpool.Config.
type config struct {
	maxConns          int32
	minConns          int32
	maxConnLifetime   time.Duration
	maxConnIdleTime   time.Duration
	healthCheckPeriod time.Duration
	connectTimeout    time.Duration

	tracer pgx.QueryTracer

	retry retryConfig
}

// defaults returns a config carrying the resilient baseline. Every field has a
// usable value before any Option runs.
func defaults() config {
	return config{
		maxConns:          defaultMaxConns(),
		minConns:          defaultMinConns,
		maxConnLifetime:   defaultMaxConnLifetime,
		maxConnIdleTime:   defaultMaxConnIdleTime,
		healthCheckPeriod: defaultHealthCheckPeriod,
		connectTimeout:    defaultConnectTimeout,
		retry:             defaultRetryConfig(),
	}
}

// applyTo maps the resolved config onto a parsed pgxpool.Config in place.
func (c *config) applyTo(pc *pgxpool.Config) {
	pc.MaxConns = c.maxConns
	pc.MinConns = c.minConns
	pc.MaxConnLifetime = c.maxConnLifetime
	pc.MaxConnIdleTime = c.maxConnIdleTime
	pc.HealthCheckPeriod = c.healthCheckPeriod
	pc.ConnConfig.ConnectTimeout = c.connectTimeout
	if c.tracer != nil {
		pc.ConnConfig.Tracer = c.tracer
	}
}

// Option configures a DB. Options are applied in order by New; a later Option
// wins over an earlier one.
type Option func(*config)

// WithMaxConns sets the maximum number of connections in the pool. A
// non-positive size is ignored, keeping the resilient default.
func WithMaxConns(n int32) Option {
	return func(c *config) {
		if n > 0 {
			c.maxConns = n
		}
	}
}

// WithMinConns sets the minimum number of idle connections the pool keeps warm.
// A negative value is ignored; zero is a valid setting (no warm minimum).
func WithMinConns(n int32) Option {
	return func(c *config) {
		if n >= 0 {
			c.minConns = n
		}
	}
}

// WithMaxConnLifetime caps how long a connection may live before it is retired
// and replaced. A non-positive duration keeps the default.
func WithMaxConnLifetime(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.maxConnLifetime = d
		}
	}
}

// WithMaxConnIdleTime caps how long a connection may sit idle before it is
// closed. A non-positive duration keeps the default.
func WithMaxConnIdleTime(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.maxConnIdleTime = d
		}
	}
}

// WithHealthCheckPeriod sets how often the pool checks the health of idle
// connections. A non-positive duration keeps the default.
func WithHealthCheckPeriod(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.healthCheckPeriod = d
		}
	}
}

// WithConnectTimeout bounds how long a single connection attempt may take. A
// non-positive duration keeps the default.
func WithConnectTimeout(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.connectTimeout = d
		}
	}
}

// WithTracer installs a pgx QueryTracer on every connection, enabling query
// tracing without pulling a telemetry library into the core. For OpenTelemetry,
// use the otel subpackage, which supplies a tracer through this seam. A nil
// tracer is ignored.
func WithTracer(t pgx.QueryTracer) Option {
	return func(c *config) {
		if t != nil {
			c.tracer = t
		}
	}
}

// WithTxRetry sets the default transaction retry policy RunInTx applies:
// attempts is the maximum number of tries (not extra retries), and base/max
// bound the exponential backoff between them. attempts below 1 and non-positive
// backoff bounds are ignored, keeping the defaults. A per-call TxOption
// (WithAttempts, WithBackoff) overrides this.
func WithTxRetry(attempts int, base, max time.Duration) Option {
	return func(c *config) {
		if attempts >= 1 {
			c.retry.attempts = attempts
		}
		if base > 0 {
			c.retry.baseBackoff = base
		}
		if max > 0 {
			c.retry.maxBackoff = max
		}
	}
}

// txConfig is the resolved per-call transaction configuration a TxOption
// mutates: the pgx transaction options and the retry policy for this call.
type txConfig struct {
	txOpts pgx.TxOptions
	retry  retryConfig
}

// TxOption configures a single RunInTx call. Options are applied in order; a
// later Option wins over an earlier one.
type TxOption func(*txConfig)

// WithTxOptions sets the pgx transaction options (isolation level, access mode,
// deferrable) for this call. Use it to run under Serializable isolation, where
// serialization failures -- which RunInTx retries -- are expected under
// contention.
func WithTxOptions(o pgx.TxOptions) TxOption {
	return func(c *txConfig) { c.txOpts = o }
}

// WithIsolation sets only the transaction isolation level for this call, leaving
// the other pgx transaction options unchanged.
func WithIsolation(level pgx.TxIsoLevel) TxOption {
	return func(c *txConfig) { c.txOpts.IsoLevel = level }
}

// WithAttempts overrides the maximum number of tries for this call. A value
// below 1 is ignored, keeping the DB default.
func WithAttempts(n int) TxOption {
	return func(c *txConfig) {
		if n >= 1 {
			c.retry.attempts = n
		}
	}
}

// WithBackoff overrides the exponential backoff bounds for this call.
// Non-positive bounds are ignored, keeping the DB default.
func WithBackoff(base, max time.Duration) TxOption {
	return func(c *txConfig) {
		if base > 0 {
			c.retry.baseBackoff = base
		}
		if max > 0 {
			c.retry.maxBackoff = max
		}
	}
}
