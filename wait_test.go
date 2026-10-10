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
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// closedPort returns a local TCP port nothing listens on.
func closedPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return strconv.Itoa(port)
}

// sqlStateErr is an error carrying a SQLSTATE the way lib/pq's *pq.Error does
// (golang-migrate's postgres driver returns those), without importing it.
type sqlStateErr string

func (e sqlStateErr) Error() string    { return "pq: " + string(e) }
func (e sqlStateErr) SQLState() string { return string(e) }

func TestTransient(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"refused", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, true},
		{"dns lookup", &net.DNSError{Err: "server misbehaving", Name: "db.example.org", IsTemporary: true}, true},
		{"dns not found", &net.DNSError{Err: "no such host", Name: "db.example.org", IsNotFound: true}, true},
		{"wrapped refused", fmt.Errorf("postgres: ping on connect: %w", syscall.ECONNREFUSED), true},
		{"reset", fmt.Errorf("read: %w", syscall.ECONNRESET), true},
		{"unexpected eof", fmt.Errorf("read: %w", io.ErrUnexpectedEOF), true},
		{"connect timeout", fmt.Errorf("dial: %w", context.DeadlineExceeded), true},
		{"starting up", &pgconn.PgError{Code: "57P03"}, true},
		{"admin shutdown", &pgconn.PgError{Code: "57P01"}, true},
		{"crash shutdown", &pgconn.PgError{Code: "57P02"}, true},
		{"too many connections", &pgconn.PgError{Code: "53300"}, true},
		{"connection exception", &pgconn.PgError{Code: "08006"}, true},
		{"lib/pq starting up", fmt.Errorf("failed to open database: %w", sqlStateErr("57P03")), true},
		{"bad password", &pgconn.PgError{Code: "28P01"}, false},
		{"no database", &pgconn.PgError{Code: "3D000"}, false},
		{"syntax error", &pgconn.PgError{Code: "42601"}, false},
		{"lib/pq bad password", sqlStateErr("28P01"), false},
		{"canceled", context.Canceled, false},
		{"plain", errors.New("migration 3 failed"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Transient(c.err); got != c.want {
				t.Fatalf("Transient(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

// The errors New and Migrate really return while a server is unreachable are
// transient, however deeply pgx, lib/pq and golang-migrate wrap them.
func TestTransient_RealUnreachableErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1_init.up.sql"), []byte("SELECT 1;"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for name, dsn := range map[string]string{
		"refused": "postgres://app@127.0.0.1:" + closedPort(t) + "/app?sslmode=disable&connect_timeout=2",
		"dns":     "postgres://app@db.invalid:5432/app?sslmode=disable&connect_timeout=2",
	} {
		t.Run(name+"/New", func(t *testing.T) {
			_, err := New(ctx, dsn)
			if err == nil || !Transient(err) {
				t.Fatalf("New error %v: Transient = false, want true", err)
			}
		})
		t.Run(name+"/Migrate", func(t *testing.T) {
			err := Migrate(dsn, dir)
			if err == nil || !Transient(err) {
				t.Fatalf("Migrate error %v: Transient = false, want true", err)
			}
		})
	}
}

func TestWaitFor_RetriesTransientUntilSuccess(t *testing.T) {
	calls := 0
	var hooked []int
	err := WaitFor(context.Background(), func(context.Context) error {
		calls++
		if calls < 4 {
			return syscall.ECONNREFUSED
		}
		return nil
	}, WithWaitBackoff(time.Millisecond, 4*time.Millisecond), WithRetryHook(func(attempt int, err error, delay time.Duration) {
		if !errors.Is(err, syscall.ECONNREFUSED) || delay <= 0 || delay > 4*time.Millisecond {
			t.Errorf("hook(%d, %v, %v)", attempt, err, delay)
		}
		hooked = append(hooked, attempt)
	}))
	if err != nil {
		t.Fatalf("WaitFor = %v", err)
	}
	if calls != 4 || len(hooked) != 3 || hooked[0] != 1 || hooked[2] != 3 {
		t.Fatalf("calls = %d, hook attempts = %v", calls, hooked)
	}
}

func TestWaitFor_PermanentErrorReturnsAtOnce(t *testing.T) {
	calls := 0
	bad := &pgconn.PgError{Code: "28P01"}
	err := WaitFor(context.Background(), func(context.Context) error { calls++; return bad },
		WithWaitBackoff(time.Millisecond, time.Millisecond))
	if !errors.Is(err, bad) || calls != 1 {
		t.Fatalf("WaitFor = %v after %d calls, want the error after 1", err, calls)
	}
}

func TestWaitFor_StopsWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := WaitFor(ctx, func(context.Context) error { return syscall.ECONNREFUSED },
		WithWaitBackoff(5*time.Millisecond, 10*time.Millisecond))
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, syscall.ECONNREFUSED) {
		t.Fatalf("WaitFor = %v, want the context error and the last attempt's error", err)
	}
}

func TestWaitDelay_JitterStaysInTheBackoffWindow(t *testing.T) {
	for attempt := 1; attempt < 40; attempt++ {
		full := backoff(attempt, 100*time.Millisecond, 2*time.Second)
		d := waitDelay(attempt, 100*time.Millisecond, 2*time.Second)
		if d < full/2 || d > full {
			t.Fatalf("attempt %d: delay %v outside [%v, %v]", attempt, d, full/2, full)
		}
	}
}
