package httpapi

import (
	"log"
	"net/http"
	"time"
)

// NewHandler returns the production handler (mux + observability middleware).
//
// Tests can still use NewMux directly to avoid noisy logs unless needed.
func NewHandler() http.Handler {
	return NewHandlerWithOptions(Options{})
}

func NewHandlerWithOptions(opt Options) http.Handler {
	return withObservability(NewMuxWithOptions(opt))
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func withObservability(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)

		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}

		pattern := r.Pattern
		if pattern == "" {
			// Keep it low-cardinality; avoid logging/querying RawQuery because it may contain secrets.
			pattern = r.Method + " " + r.URL.Path
		}

		metricsIncRequest(pattern, status)

		// Minimal access log. Keep it safe: never log the query string.
		if r.URL.Path != "/healthz" && r.URL.Path != "/metrics" {
			dur := time.Since(start).Round(time.Millisecond)
			log.Printf("http %s %s pattern=%q status=%d dur=%s bytes=%d", r.Method, r.URL.Path, pattern, status, dur, sw.bytes)
		}
	})
}
