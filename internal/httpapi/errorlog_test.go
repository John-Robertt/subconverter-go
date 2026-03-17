package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/John-Robertt/subconverter-go/internal/errlog"
	"github.com/John-Robertt/subconverter-go/internal/model"
)

func TestMux_Healthz_UnhealthyWhenErrorLogDirUnavailable(t *testing.T) {
	store, err := errlog.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	mux := NewMuxWithOptions(Options{ErrorLog: store})

	if err := os.RemoveAll(store.Dir()); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if got, want := rr.Code, http.StatusServiceUnavailable; got != want {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "unhealthy: error-log-dir unavailable") {
		t.Fatalf("body=%q", rr.Body.String())
	}
}

func TestMux_Healthz_DoesNotCreateBusinessLogs(t *testing.T) {
	store, err := errlog.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	mux := NewMuxWithOptions(Options{ErrorLog: store})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if got, want := rr.Code, http.StatusOK; got != want {
			t.Fatalf("healthz status=%d body=%q", rr.Code, rr.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/logs/errors.zip", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if got, want := rr.Code, http.StatusNotFound; got != want {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}

	var resp model.ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if resp.Error.Code != "LOG_NOT_FOUND" {
		t.Fatalf("code=%q, want=LOG_NOT_FOUND", resp.Error.Code)
	}

	entries, err := os.ReadDir(store.Dir())
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "errors-") && strings.HasSuffix(entry.Name(), ".jsonl") {
			t.Fatalf("unexpected log file created by healthz: %s", entry.Name())
		}
	}
}

func TestMux_Healthz_RecoversAfterDirRestored(t *testing.T) {
	store, err := errlog.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	mux := NewMuxWithOptions(Options{ErrorLog: store})

	if err := os.RemoveAll(store.Dir()); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if got, want := rr.Code, http.StatusServiceUnavailable; got != want {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}

	if err := os.MkdirAll(store.Dir(), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if got, want := rr.Code, http.StatusOK; got != want {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
}

func TestConvertFailure_WithUnavailableErrorLogDirPreservesOriginalResponse(t *testing.T) {
	store, err := errlog.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	mux := NewMuxWithOptions(Options{ErrorLog: store})

	if err := os.RemoveAll(store.Dir()); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/sub", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if got, want := rr.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if got, want := rr.Code, http.StatusServiceUnavailable; got != want {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}

	if err := os.MkdirAll(store.Dir(), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if got, want := rr.Code, http.StatusOK; got != want {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
}

func TestMux_ErrorLogsZip_OK(t *testing.T) {
	store, err := errlog.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	rec := errlog.FailureRecord{
		SchemaVersion: 1,
		TS:            time.Date(2026, 3, 17, 8, 0, 0, 0, time.UTC),
		RequestID:     "req-zip",
		HTTP:          errlog.HTTPInfo{Method: http.MethodGet, Path: "/sub"},
		Error: errlog.FailureError{
			Status: 400,
			App: model.AppError{
				Code:    "INVALID_ARGUMENT",
				Message: "缺少 mode 参数",
				Stage:   "validate_request",
			},
			Cause: errors.New("missing mode").Error(),
		},
	}
	if err := store.WriteFailure(rec); err != nil {
		t.Fatalf("WriteFailure: %v", err)
	}

	mux := NewMuxWithOptions(Options{ErrorLog: store})
	req := httptest.NewRequest(http.MethodGet, "/logs/errors.zip", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if got, want := rr.Code, http.StatusOK; got != want {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("Content-Type=%q", got)
	}

	zr, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	if len(zr.File) != 1 {
		t.Fatalf("zip entries=%d, want=1", len(zr.File))
	}
	if zr.File[0].Name != "errors-2026-03-17.jsonl" {
		t.Fatalf("zip name=%q", zr.File[0].Name)
	}
}

func TestConvertFailure_WritesErrorSnapshot(t *testing.T) {
	store, err := errlog.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	mux := NewMuxWithOptions(Options{ErrorLog: store})

	req := httptest.NewRequest(http.MethodGet, "/sub", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if got, want := rr.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}

	record := mustReadSingleFailureRecord(t, store.Dir())
	if record.Error.App.Stage != "validate_request" {
		t.Fatalf("stage=%q, want=validate_request", record.Error.App.Stage)
	}
	if record.HTTP.Path != "/sub" {
		t.Fatalf("path=%q", record.HTTP.Path)
	}
	if record.RequestID == "" {
		t.Fatalf("request_id should not be empty")
	}
}

func TestSuccessfulConvert_DoesNotWriteErrorSnapshot(t *testing.T) {
	store, err := errlog.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	mux := NewMuxWithOptions(Options{ErrorLog: store})

	subBody := "ss://YWVzLTEyOC1nY206cGFzc3dvcmQ=@example.com:8388#A\n"
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, subBody)
	}))
	defer up.Close()

	path := "/sub?mode=list&encode=raw&sub=" + url.QueryEscape(up.URL)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if got, want := rr.Code, http.StatusOK; got != want {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}

	entries, err := os.ReadDir(store.Dir())
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "errors-") && strings.HasSuffix(entry.Name(), ".jsonl") {
			t.Fatalf("unexpected log file: %s", entry.Name())
		}
	}
}

func TestProfileParseFailure_CapturesProfileSnapshot(t *testing.T) {
	store, err := errlog.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	mux := NewMuxWithOptions(Options{ErrorLog: store})

	subBody := "ss://YWVzLTEyOC1nY206cGFzc3dvcmQ=@example.com:8388#A\n"
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sub":
			_, _ = fmt.Fprint(w, subBody)
		case "/profile":
			_, _ = fmt.Fprint(w, "version: [broken")
		default:
			http.NotFound(w, r)
		}
	}))
	defer up.Close()

	path := "/sub?mode=config&target=clash&sub=" + url.QueryEscape(up.URL+"/sub") + "&profile=" + url.QueryEscape(up.URL+"/profile")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if got, want := rr.Code, http.StatusUnprocessableEntity; got != want {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}

	record := mustReadSingleFailureRecord(t, store.Dir())
	foundProfile := false
	for _, resource := range record.Resources {
		if resource.Kind != "profile" {
			continue
		}
		foundProfile = true
		if resource.Preview == "" {
			t.Fatalf("profile preview should not be empty")
		}
	}
	if !foundProfile {
		t.Fatalf("expected profile snapshot in resources")
	}
}

func mustReadSingleFailureRecord(t *testing.T, dir string) errlog.FailureRecord {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(dir, "errors-*.jsonl"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches=%d, want=1 (%v)", len(matches), matches)
	}
	body, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 1 {
		t.Fatalf("lines=%d, want=1", len(lines))
	}
	var rec errlog.FailureRecord
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return rec
}
