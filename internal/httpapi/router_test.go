package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/John-Robertt/subconverter-go/internal/model"
)

func TestMux_Healthz_OK(t *testing.T) {
	mux := NewMux()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if got, want := rr.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := rr.Body.String(), "ok\n"; got != want {
		t.Fatalf("body=%q, want=%q", got, want)
	}
}

func TestMux_Sub_MissingMode_400(t *testing.T) {
	mux := NewMux()
	req := httptest.NewRequest(http.MethodGet, "/sub", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if got, want := rr.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}

	var resp model.ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody=%q", err, rr.Body.String())
	}
	if resp.Error.Code != "INVALID_ARGUMENT" {
		t.Fatalf("code = %q, want %q", resp.Error.Code, "INVALID_ARGUMENT")
	}
	if resp.Error.Stage != "validate_request" {
		t.Fatalf("stage = %q, want %q", resp.Error.Stage, "validate_request")
	}
}
