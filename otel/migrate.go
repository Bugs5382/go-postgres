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
	"context"
	"time"

	golog "github.com/Bugs5382/go-log"
	gootel "github.com/Bugs5382/go-otel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// tracerName scopes the span InstrumentMigrate starts. Consumers who want a
// distinct instrumentation scope can start their own span and pass its ctx in.
const tracerName = "github.com/Bugs5382/go-postgres/otel"

// InstrumentMigrate wraps a migration run -- typically postgres.Migrate or
// postgres.MigrateWithTable -- with the same observability seam WithTracing
// gives query traffic: a "postgres.migrate" span, a duration histogram and a
// count (both built with github.com/Bugs5382/go-otel's instrument
// constructors) off the global MeterProvider, and structured,
// trace-correlated logs via github.com/Bugs5382/go-log. name identifies the
// migration set in the span, the metrics, and the logs (for example "app" or
// the migrations table name); it distinguishes services that run more than
// one migration set, for example with MigrateWithTable. Call Init (from
// github.com/Bugs5382/go-otel) and log.New before this, so spans, metrics,
// and logs land on a configured provider; without it they are safely
// dropped/no-op, but run is still called and its result still returned.
//
//	err := otelpg.InstrumentMigrate(ctx, "app", func() error {
//		return postgres.Migrate(dsn, "./migrations")
//	})
func InstrumentMigrate(ctx context.Context, name string, run func() error) error {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "postgres.migrate",
		trace.WithAttributes(attribute.String("migrate.name", name)),
	)
	defer span.End()

	logger := golog.Ctx(ctx)
	logger.Info().Str("migrate.name", name).Msg("running migrations")

	start := time.Now()
	err := run()
	elapsed := time.Since(start)

	attrs := metric.WithAttributes(attribute.String("migrate.name", name))
	gootel.Histogram("postgres.migrate.duration", "Duration of a go-postgres migration run.", "s").
		Record(ctx, elapsed.Seconds(), attrs)
	gootel.Counter("postgres.migrate.count", "Count of go-postgres migration runs, by outcome.").
		Add(ctx, 1, metric.WithAttributes(
			attribute.String("migrate.name", name),
			attribute.Bool("error", err != nil),
		))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error().Err(err).Str("migrate.name", name).Dur("duration", elapsed).Msg("migration failed")
		return err
	}

	logger.Info().Str("migrate.name", name).Dur("duration", elapsed).Msg("migrations complete")
	return nil
}
