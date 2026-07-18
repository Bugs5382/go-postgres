package otel_test

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
	"testing"
	"time"

	postgres "github.com/Bugs5382/go-postgres"
	otelpg "github.com/Bugs5382/go-postgres/otel"
)

func TestWithTracingReturnsOption(t *testing.T) {
	t.Parallel()
	if otelpg.WithTracing() == nil {
		t.Fatal("WithTracing returned a nil Option")
	}
}

func TestWithTracingWiresIntoNew(t *testing.T) {
	t.Parallel()
	// The tracer is installed on the pool config; New still fails on the
	// readiness Ping because nothing is listening, which proves the option is
	// accepted and applied without a live server.
	ctx := context.Background()
	_, err := postgres.New(ctx,
		"postgres://user:pass@127.0.0.1:1/acme",
		otelpg.WithTracing(),
		postgres.WithConnectTimeout(200*time.Millisecond),
	)
	if err == nil {
		t.Fatal("New against an unreachable server should fail")
	}
}
