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
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testDSN = "postgres://user:pass@localhost:5432/acme"

// apply builds a config from defaults and the given options, as New does.
func apply(opts ...Option) config {
	c := defaults()
	for _, o := range opts {
		o(&c)
	}
	return c
}

func TestDefaults(t *testing.T) {
	t.Parallel()
	c := defaults()
	if c.maxConns < 4 {
		t.Errorf("maxConns = %d, want >= 4", c.maxConns)
	}
	if c.minConns != defaultMinConns {
		t.Errorf("minConns = %d, want %d", c.minConns, defaultMinConns)
	}
	if c.maxConnLifetime != defaultMaxConnLifetime || c.maxConnIdleTime != defaultMaxConnIdleTime {
		t.Errorf("lifetimes = %v/%v, want defaults", c.maxConnLifetime, c.maxConnIdleTime)
	}
	if c.healthCheckPeriod != defaultHealthCheckPeriod || c.connectTimeout != defaultConnectTimeout {
		t.Errorf("periods = %v/%v, want defaults", c.healthCheckPeriod, c.connectTimeout)
	}
	if c.tracer != nil {
		t.Error("tracer should default to nil")
	}
	if c.retry.attempts != defaultTxAttempts ||
		c.retry.baseBackoff != defaultTxBaseBackoff || c.retry.maxBackoff != defaultTxMaxBackoff {
		t.Errorf("retry = %+v, want defaults", c.retry)
	}
}

func TestScalarOptions(t *testing.T) {
	t.Parallel()
	c := apply(
		WithMaxConns(50),
		WithMinConns(5),
		WithMaxConnLifetime(2*time.Hour),
		WithMaxConnIdleTime(15*time.Minute),
		WithHealthCheckPeriod(30*time.Second),
		WithConnectTimeout(9*time.Second),
		WithTxRetry(6, 20*time.Millisecond, 2*time.Second),
	)
	if c.maxConns != 50 || c.minConns != 5 {
		t.Errorf("conns not applied: max=%d min=%d", c.maxConns, c.minConns)
	}
	if c.maxConnLifetime != 2*time.Hour || c.maxConnIdleTime != 15*time.Minute {
		t.Errorf("lifetimes not applied: %v/%v", c.maxConnLifetime, c.maxConnIdleTime)
	}
	if c.healthCheckPeriod != 30*time.Second || c.connectTimeout != 9*time.Second {
		t.Errorf("periods not applied: %v/%v", c.healthCheckPeriod, c.connectTimeout)
	}
	if c.retry.attempts != 6 || c.retry.baseBackoff != 20*time.Millisecond || c.retry.maxBackoff != 2*time.Second {
		t.Errorf("retry not applied: %+v", c.retry)
	}
}

func TestNonPositiveValuesKeepDefaults(t *testing.T) {
	t.Parallel()
	d := defaults()
	c := apply(
		WithMaxConns(0),
		WithMinConns(-1),
		WithMaxConnLifetime(0),
		WithMaxConnIdleTime(-1),
		WithHealthCheckPeriod(0),
		WithConnectTimeout(-1),
		WithTxRetry(0, 0, 0),
		WithTracer(nil),
	)
	if c.maxConns != d.maxConns || c.minConns != d.minConns {
		t.Errorf("non-positive conns changed defaults: %+v", c)
	}
	if c.maxConnLifetime != d.maxConnLifetime || c.maxConnIdleTime != d.maxConnIdleTime {
		t.Error("non-positive lifetimes should keep defaults")
	}
	if c.healthCheckPeriod != d.healthCheckPeriod || c.connectTimeout != d.connectTimeout {
		t.Error("non-positive periods should keep defaults")
	}
	if c.retry != d.retry {
		t.Errorf("invalid retry values should keep defaults: %+v", c.retry)
	}
	if c.tracer != nil {
		t.Error("WithTracer(nil) should leave the tracer unset")
	}
}

func TestMinConnsZeroIsValid(t *testing.T) {
	t.Parallel()
	// Zero is a legitimate MinConns (no warm minimum) and must be applied even
	// though it is the default, since a caller may set it deliberately.
	if c := apply(WithMinConns(0)); c.minConns != 0 {
		t.Errorf("WithMinConns(0) = %d, want 0", c.minConns)
	}
}

func TestApplyToMapsConfig(t *testing.T) {
	t.Parallel()
	pc, err := pgxpool.ParseConfig(testDSN)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	c := apply(
		WithMaxConns(42),
		WithMinConns(3),
		WithMaxConnLifetime(90*time.Minute),
		WithMaxConnIdleTime(10*time.Minute),
		WithHealthCheckPeriod(20*time.Second),
		WithConnectTimeout(4*time.Second),
	)
	c.applyTo(pc)

	if pc.MaxConns != 42 || pc.MinConns != 3 {
		t.Errorf("pool conns wrong: max=%d min=%d", pc.MaxConns, pc.MinConns)
	}
	if pc.MaxConnLifetime != 90*time.Minute || pc.MaxConnIdleTime != 10*time.Minute {
		t.Errorf("pool lifetimes wrong: %v/%v", pc.MaxConnLifetime, pc.MaxConnIdleTime)
	}
	if pc.HealthCheckPeriod != 20*time.Second {
		t.Errorf("pool health period wrong: %v", pc.HealthCheckPeriod)
	}
	if pc.ConnConfig.ConnectTimeout != 4*time.Second {
		t.Errorf("connect timeout wrong: %v", pc.ConnConfig.ConnectTimeout)
	}
	if pc.ConnConfig.Tracer != nil {
		t.Error("tracer should be nil when unset")
	}
}

func TestApplyToWiresTracer(t *testing.T) {
	t.Parallel()
	pc, err := pgxpool.ParseConfig(testDSN)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	tr := noopTracer{}
	c := apply(WithTracer(tr))
	c.applyTo(pc)
	if pc.ConnConfig.Tracer == nil {
		t.Error("WithTracer should set ConnConfig.Tracer")
	}
}

func TestTxOptionResolution(t *testing.T) {
	t.Parallel()
	txc := txConfig{retry: defaultRetryConfig()}
	for _, o := range []TxOption{
		WithIsolation(pgx.Serializable),
		WithAttempts(3),
		WithBackoff(1*time.Millisecond, 50*time.Millisecond),
	} {
		o(&txc)
	}
	if txc.txOpts.IsoLevel != pgx.Serializable {
		t.Errorf("isolation = %q, want serializable", txc.txOpts.IsoLevel)
	}
	if txc.retry.attempts != 3 {
		t.Errorf("attempts = %d, want 3", txc.retry.attempts)
	}
	if txc.retry.baseBackoff != time.Millisecond || txc.retry.maxBackoff != 50*time.Millisecond {
		t.Errorf("backoff not applied: %+v", txc.retry)
	}
}

func TestWithTxOptionsSetsWholeStruct(t *testing.T) {
	t.Parallel()
	txc := txConfig{}
	want := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	WithTxOptions(want)(&txc)
	if txc.txOpts != want {
		t.Errorf("WithTxOptions = %+v, want %+v", txc.txOpts, want)
	}
}

func TestTxOptionInvalidValuesKeepDefaults(t *testing.T) {
	t.Parallel()
	txc := txConfig{retry: defaultRetryConfig()}
	WithAttempts(0)(&txc)
	WithBackoff(0, -1)(&txc)
	if txc.retry != defaultRetryConfig() {
		t.Errorf("invalid tx options changed retry: %+v", txc.retry)
	}
}
