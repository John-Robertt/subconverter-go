package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/John-Robertt/subconverter-go/internal/model"
)

func TestMux_Sub_NotImplemented(t *testing.T) {
	mux := NewMux()
	req := httptest.NewRequest(http.MethodGet, "/sub", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if got, want := rr.Code, http.StatusInternalServerError; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}

	var resp model.ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody=%q", err, rr.Body.String())
	}
	if resp.Error.Code != "NOT_IMPLEMENTED" {
		t.Fatalf("code = %q, want %q", resp.Error.Code, "NOT_IMPLEMENTED")
	}
}
