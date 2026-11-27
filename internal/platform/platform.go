// Package platform holds the cross-cutting plumbing shared by every Go service:
// env config, structured logging, tracing, metrics and the Postgres pool.
package platform

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/propagators/aws/xray"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/mini4141/ground-station-cloud/internal/db"
)

// Env returns the environment variable or a default.
func Env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// EnvFloat returns a float environment variable or a default.
func EnvFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil {
		return v
	}
	return def
}

// InLambda reports whether the process runs inside AWS Lambda.
func InLambda() bool { return os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" }

// Setup configures slog (JSON to stdout) and OpenTelemetry tracing. The returned func flushes spans.
func Setup(ctx context.Context, service string) func() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", service))
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return func() {}
	}
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		slog.Error("otel exporter", "err", err)
		return func() {}
	}
	opts := []sdktrace.TracerProviderOption{sdktrace.WithResource(resource.NewSchemaless(semconv.ServiceName(service)))}
	if InLambda() { // ADOT collector extension -> X-Ray: needs X-Ray trace ids and synchronous export before the freeze
		opts = append(opts, sdktrace.WithIDGenerator(xray.NewIDGenerator()), sdktrace.WithSyncer(exp))
	} else {
		opts = append(opts, sdktrace.WithBatcher(exp))
	}
	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	return func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tp.Shutdown(c)
	}
}

// OpenDB connects to Postgres (traced) and applies the embedded goose migrations.
func OpenDB(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.Tracer = otelpgx.NewTracer()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	for i := 0; ; i++ { // Aurora may be waking from auto-pause
		if err = pool.Ping(ctx); err == nil {
			break
		}
		if i >= 30 {
			return nil, fmt.Errorf("ping db: %w", err)
		}
		time.Sleep(2 * time.Second)
	}
	if os.Getenv("RUN_MIGRATIONS") == "false" { // only the API Lambda migrates; avoids a cold-start race
		return pool, nil
	}
	goose.SetBaseFS(db.Migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return nil, err
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return pool, nil
}

// ServeAdmin runs a tiny HTTP server with /healthz and /metrics for non-API services.
func ServeAdmin(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"status":"ok"}`)) })
	mux.Handle("GET /metrics", promhttp.Handler())
	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			slog.Error("admin server", "err", err)
		}
	}()
}
