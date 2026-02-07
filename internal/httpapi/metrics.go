package httpapi

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// metricsStore is intentionally tiny: a few counters are enough for basic
// observability without dragging in external dependencies or complex labeling.
type metricsStore struct {
	mu sync.Mutex

	httpRequestsTotal uint64
	httpByPattern     map[reqKey]uint64

	appErrors map[errKey]uint64
}

type reqKey struct {
	Pattern string
	Status  int
}

type errKey struct {
	Stage string
	Code  string
}

func newMetricsStore() *metricsStore {
	return &metricsStore{
		httpByPattern: make(map[reqKey]uint64),
		appErrors:     make(map[errKey]uint64),
	}
}

var metrics = newMetricsStore()

func metricsIncRequest(pattern string, status int) {
	if status == 0 {
		status = http.StatusOK
	}
	if pattern == "" {
		pattern = "(unknown)"
	}

	metrics.mu.Lock()
	metrics.httpRequestsTotal++
	metrics.httpByPattern[reqKey{Pattern: pattern, Status: status}]++
	metrics.mu.Unlock()
}

func metricsIncAppError(stage, code string) {
	stage = strings.TrimSpace(stage)
	code = strings.TrimSpace(code)
	if stage == "" {
		stage = "(unknown)"
	}
	if code == "" {
		code = "(unknown)"
	}

	metrics.mu.Lock()
	metrics.appErrors[errKey{Stage: stage, Code: code}]++
	metrics.mu.Unlock()
}

type reqMetric struct {
	reqKey
	N uint64
}

type errMetric struct {
	errKey
	N uint64
}

func metricsSnapshot() (httpTotal uint64, reqs []reqMetric, errs []errMetric) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	httpTotal = metrics.httpRequestsTotal

	reqs = make([]reqMetric, 0, len(metrics.httpByPattern))
	for k, n := range metrics.httpByPattern {
		reqs = append(reqs, reqMetric{reqKey: k, N: n})
	}
	errs = make([]errMetric, 0, len(metrics.appErrors))
	for k, n := range metrics.appErrors {
		errs = append(errs, errMetric{errKey: k, N: n})
	}

	sort.Slice(reqs, func(i, j int) bool {
		if reqs[i].Pattern != reqs[j].Pattern {
			return reqs[i].Pattern < reqs[j].Pattern
		}
		return reqs[i].Status < reqs[j].Status
	})
	sort.Slice(errs, func(i, j int) bool {
		if errs[i].Stage != errs[j].Stage {
			return errs[i].Stage < errs[j].Stage
		}
		return errs[i].Code < errs[j].Code
	})
	return httpTotal, reqs, errs
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Plain text (Prometheus-ish). Keep it dependency-free.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	total, reqs, errs := metricsSnapshot()

	var b strings.Builder

	b.WriteString("# HELP subconverter_http_requests_total Total HTTP requests.\n")
	b.WriteString("# TYPE subconverter_http_requests_total counter\n")
	b.WriteString("subconverter_http_requests_total ")
	b.WriteString(strconv.FormatUint(total, 10))
	b.WriteByte('\n')

	b.WriteString("# HELP subconverter_http_requests_by_pattern_total HTTP requests by ServeMux pattern and status.\n")
	b.WriteString("# TYPE subconverter_http_requests_by_pattern_total counter\n")
	for _, m := range reqs {
		b.WriteString("subconverter_http_requests_by_pattern_total{pattern=\"")
		b.WriteString(promLabelEscape(m.Pattern))
		b.WriteString("\",status=\"")
		b.WriteString(strconv.Itoa(m.Status))
		b.WriteString("\"} ")
		b.WriteString(strconv.FormatUint(m.N, 10))
		b.WriteByte('\n')
	}

	b.WriteString("# HELP subconverter_app_errors_total Application errors returned to clients.\n")
	b.WriteString("# TYPE subconverter_app_errors_total counter\n")
	for _, m := range errs {
		b.WriteString("subconverter_app_errors_total{stage=\"")
		b.WriteString(promLabelEscape(m.Stage))
		b.WriteString("\",code=\"")
		b.WriteString(promLabelEscape(m.Code))
		b.WriteString("\"} ")
		b.WriteString(strconv.FormatUint(m.N, 10))
		b.WriteByte('\n')
	}

	_, _ = fmt.Fprint(w, b.String())
}

func promLabelEscape(s string) string {
	// Prometheus label value escaping: backslash and double quote.
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
