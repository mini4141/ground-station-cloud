package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/mini4141/ground-station-cloud/internal/api"
	"github.com/mini4141/ground-station-cloud/internal/auth"
	"github.com/mini4141/ground-station-cloud/internal/storage"
)

var httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name: "gs_http_request_duration_seconds", Help: "API request latency.", Buckets: prometheus.DefBuckets,
}, []string{"method", "route", "code"})

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (s *statusWriter) WriteHeader(c int) { s.code = c; s.ResponseWriter.WriteHeader(c) }

func metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start, sw := time.Now(), &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)
		httpDuration.WithLabelValues(r.Method, r.Pattern, strconv.Itoa(sw.code)).Observe(time.Since(start).Seconds())
	})
}

func cors(origin string, next http.Handler) http.Handler {
	if origin == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Upload-Token")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func public(r *http.Request) bool {
	p := r.URL.Path
	return p == "/healthz" || p == "/metrics" || strings.HasPrefix(p, "/uploads/") || strings.HasSuffix(p, "/upload-url") || r.Method == http.MethodOptions
}

// Handler assembles routing, auth, metrics, tracing and (locally) CORS and the signed upload endpoint.
func (s *Server) Handler(v *auth.Verifier, local *storage.Local, corsOrigin string) http.Handler {
	mux := http.NewServeMux()
	api.HandlerWithOptions(s, api.StdHTTPServerOptions{BaseRouter: mux, Middlewares: []api.MiddlewareFunc{metrics}})
	mux.Handle("GET /metrics", promhttp.Handler())
	if local != nil {
		mux.Handle("PUT /uploads/{key...}", local.Handler())
	}
	var h http.Handler = v.Middleware(public, mux)
	h = cors(corsOrigin, h)
	return otelhttp.NewHandler(h, "api", otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string { return r.Method + " " + r.URL.Path }))
}
