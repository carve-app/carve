package metrics

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/chi/v5"
)

// statusRecorder captures the HTTP status so we can record errors vs. successes.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// HTTPMiddleware records latency + status for every request and propagates
// the chi request-ID into every slog log line in the downstream handler chain.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}

		// Inject request-ID into the slog context so downstream handlers can
		// use slog.InfoContext / slog.ErrorContext and emit the rid label.
		rid := chimw.GetReqID(r.Context())
		ctx := withRequestID(r.Context(), rid)

		next.ServeHTTP(rec, r.WithContext(ctx))

		dur := time.Since(start)
		// Use the chi-resolved pattern so /v1/cards/{id} doesn't blow up the
		// label cardinality with one entry per card ID.
		path := chi.RouteContext(r.Context()).RoutePattern()
		if path == "" {
			path = r.URL.Path
		}
		IncRequest(r.Method, path, rec.status, dur)

		if rec.status >= 500 {
			slog.Error("request",
				"rid", rid, "method", r.Method, "path", path,
				"status", rec.status, "dur_ms", dur.Milliseconds())
		}
	})
}

type ridKey struct{}

func withRequestID(ctx context.Context, rid string) context.Context {
	return context.WithValue(ctx, ridKey{}, rid)
}

// RequestIDFromContext returns the request ID injected by HTTPMiddleware, or
// the empty string if the middleware wasn't on this code path.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ridKey{}).(string); ok {
		return v
	}
	return ""
}
