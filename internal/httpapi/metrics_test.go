package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetrics_CountsRequestsAndErrors(t *testing.T) {
	metrics = newMetricsStore()
	h := NewHandler()

	// 1) ok request
	{
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("healthz status=%d body=%q", rr.Code, rr.Body.String())
		}
	}

	// 2) error request
	{
		req := httptest.NewRequest(http.MethodGet, "/sub", nil) // missing mode => validate_request error
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("sub status=%d body=%q", rr.Code, rr.Body.String())
		}
	}

	// 3) metrics snapshot (note: /metrics request itself isn't counted inside its own response).
	{
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("metrics status=%d body=%q", rr.Code, rr.Body.String())
		}

		body := rr.Body.String()

		if !strings.Contains(body, "subconverter_http_requests_total 2\n") {
			t.Fatalf("metrics body missing total requests=2, got:\n%s", body)
		}
		if !strings.Contains(body, `pattern="GET /healthz",status="200"} 1`) {
			t.Fatalf("metrics body missing healthz counter, got:\n%s", body)
		}
		if !strings.Contains(body, `pattern="GET /sub",status="400"} 1`) {
			t.Fatalf("metrics body missing sub 400 counter, got:\n%s", body)
		}
		if !strings.Contains(body, `subconverter_app_errors_total{stage="validate_request",code="INVALID_ARGUMENT"} 1`) {
			t.Fatalf("metrics body missing app error counter, got:\n%s", body)
		}
	}
}
