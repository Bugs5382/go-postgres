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
	"errors"
	"testing"

	otelpg "github.com/Bugs5382/go-postgres/otel"
)

// Against the default no-op global providers, InstrumentMigrate still runs the
// wrapped function and returns its result -- the span, metrics, and logging
// seam must never gate the migration itself.

func TestInstrumentMigrateSuccess(t *testing.T) {
	t.Parallel()
	ran := false
	err := otelpg.InstrumentMigrate(context.Background(), "app", func() error {
		ran = true
		return nil
	})
	if err != nil {
		t.Fatalf("InstrumentMigrate() = %v, want nil", err)
	}
	if !ran {
		t.Fatal("InstrumentMigrate did not call run")
	}
}

func TestInstrumentMigratePropagatesError(t *testing.T) {
	t.Parallel()
	want := errors.New("migrate up: boom")
	err := otelpg.InstrumentMigrate(context.Background(), "app", func() error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("InstrumentMigrate() = %v, want %v", err, want)
	}
}
