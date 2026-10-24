// Package otel adds optional OpenTelemetry query tracing to a go-postgres DB. It
// wraps github.com/exaring/otelpgx, which records a span per query against the
// global TracerProvider, and exposes it as a go-postgres Option so it plugs into
// the same New call. Keeping it in a subpackage means the core go-postgres library
// carries no OpenTelemetry dependency; import this only when you want it.
//
// InstrumentMigrate extends the same seam to postgres.Migrate,
// postgres.MigrateWithTable and their fs.FS forms: a span, a duration
// histogram, a count, and structured, trace-correlated logs (via
// github.com/Bugs5382/go-log) around a migration run, using the same global TracerProvider/MeterProvider as
// WithTracing (for example set up with github.com/Bugs5382/go-otel).
package otel

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
