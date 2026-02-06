package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/John-Robertt/subconverter-go/internal/model"
)

func TestWriteError_JSONShapeAndHeaders(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteError(rr, http.StatusUnprocessableEntity, model.AppError{
		Code:    "RULE_PARSE_ERROR",
		Message: "invalid rule line",
		Stage:   "parse_ruleset",
		URL:     "https://example.com/Proxy.list",
		Line:    123,
		Snippet: "DOMAIN-SUFFIX,google.com",
		Hint:    "expected: TYPE,VALUE[,ACTION][,no-resolve]",
	})

	if got, want := rr.Code, http.StatusUnprocessableEntity; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}

	if got, want := rr.Header().Get("Content-Type"), "application/json; charset=utf-8"; got != want {
		t.Fatalf("Content-Type = %q, want %q", got, want)
	}

	var resp model.ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody=%q", err, rr.Body.String())
	}
	if resp.Error.Code != "RULE_PARSE_ERROR" {
		t.Fatalf("code = %q, want %q", resp.Error.Code, "RULE_PARSE_ERROR")
	}
	if resp.Error.Stage != "parse_ruleset" {
		t.Fatalf("stage = %q, want %q", resp.Error.Stage, "parse_ruleset")
	}
	if resp.Error.Line != 123 {
		t.Fatalf("line = %d, want %d", resp.Error.Line, 123)
	}
}
