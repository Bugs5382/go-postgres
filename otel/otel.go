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

import (
	postgres "github.com/Bugs5382/go-postgres"
	"github.com/exaring/otelpgx"
)

// WithTracing returns a go-postgres Option that installs an OpenTelemetry query
// tracer on every pooled connection. Spans are recorded against the global
// TracerProvider, so configure that (for example with github.com/Bugs5382/go-otel)
// before serving traffic. Any otelpgx options are forwarded to the tracer. Pass
// the result straight to postgres.New:
//
//	db, err := postgres.New(ctx, dsn, otel.WithTracing())
func WithTracing(opts ...otelpgx.Option) postgres.Option {
	return postgres.WithTracer(otelpgx.NewTracer(opts...))
}
