package errlog

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/John-Robertt/subconverter-go/internal/model"
)

func TestNewStore_DefaultsToWorkingDir(t *testing.T) {
	tmp := t.TempDir()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWD)
	}()

	store, err := NewStore("")
	if err != nil {
		t.Fatalf("NewStore empty dir: %v", err)
	}
	got, err := filepath.EvalSymlinks(store.Dir())
	if err != nil {
		t.Fatalf("EvalSymlinks(store.Dir): %v", err)
	}
	want, err := filepath.EvalSymlinks(filepath.Clean(tmp))
	if err != nil {
		t.Fatalf("EvalSymlinks(tmp): %v", err)
	}
	if got != want {
		t.Fatalf("store.Dir()=%q, want=%q", got, want)
	}
}

func TestValidateStartup_CreatesMissingDirWithoutBusinessLogs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs", "nested")
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore missing dir: %v", err)
	}

	if err := store.ValidateStartup(time.Date(2026, 3, 17, 8, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ValidateStartup: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("stat created dir: %v", err)
	}
	assertNoBusinessLogFiles(t, dir)
}

func TestHealth_HealthyDoesNotCreateBusinessLogs(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	for i := 0; i < 2; i++ {
		status := store.Health(time.Date(2026, 3, 17, 8, 0, i, 0, time.UTC))
		if !status.Healthy {
			t.Fatalf("Health()=%+v, want healthy", status)
		}
	}

	assertNoBusinessLogFiles(t, store.Dir())
}

func TestWriteFailure_JSONLAndRedaction(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	req := httptest.NewRequest("GET", "/sub", nil)
	collector := NewCollector("req-1", req)
	collector.SetRequest("config", "clash", []string{"https://example.com/sub?a=1&token=secret"}, "https://example.com/profile.yaml?token=secret", "clash", "")
	collector.AddResource(NewResourceSnapshot(ResourceSubscription, "https://example.com/sub?a=1&token=secret", "ss://YWVzLTEyOC1nY206cGFzcw@example.com:8388#A\n"))
	collector.AddResource(NewResourceSnapshot(ResourceProfile, "https://example.com/profile.yaml?token=secret", "template:\n  clash: https://tmpl.example.com/clash.yaml?sig=abc\n"))
	collector.SetParsedProxyCount(2)
	collector.SetCompiledCounts(2, 1, 3)

	rec := collector.BuildFailure(
		time.Date(2026, 3, 17, 8, 30, 0, 0, time.UTC),
		422,
		model.AppError{
			Code:    "PROFILE_PARSE_ERROR",
			Message: "profile YAML 解析失败",
			Stage:   "parse_profile",
			URL:     "https://example.com/profile.yaml?token=secret",
			Hint:    "see https://docs.example.com/fix?token=secret",
		},
		errors.New(`Get "https://example.com/profile.yaml?token=secret": bad gateway`),
	)

	if err := store.WriteFailure(rec); err != nil {
		t.Fatalf("WriteFailure: %v", err)
	}

	path := filepath.Join(store.Dir(), "errors-2026-03-17.jsonl")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 1 {
		t.Fatalf("lines=%d, want=1", len(lines))
	}

	var got FailureRecord
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Error.App.URL != "https://example.com/profile.yaml?token=%3Credacted%3E" {
		t.Fatalf("app url=%q", got.Error.App.URL)
	}
	if !strings.Contains(got.Error.Cause, "%3Credacted%3E") {
		t.Fatalf("cause not redacted: %q", got.Error.Cause)
	}
	if len(got.Resources) != 2 {
		t.Fatalf("resources=%d, want=2", len(got.Resources))
	}
	if got.Resources[0].Preview != "" {
		t.Fatalf("subscription preview should be empty, got=%q", got.Resources[0].Preview)
	}
	if !got.Resources[0].Redacted {
		t.Fatalf("subscription resource should be marked redacted")
	}
	if !strings.Contains(got.Resources[1].Preview, "%3Credacted%3E") {
		t.Fatalf("profile preview should redact query values, got=%q", got.Resources[1].Preview)
	}
}

func TestWriteFailureFailure_DegradesStoreUntilProbeRecovers(t *testing.T) {
	dir := t.TempDir()
	failLogWrite := true
	failProbeWrite := true
	store := newStoreWithFS(dir, faultFS{
		base: osFS{},
		openFile: func(name string, flag int, perm os.FileMode) (fileHandle, error) {
			handle, err := osFS{}.OpenFile(name, flag, perm)
			if err != nil {
				return nil, err
			}

			base := filepath.Base(name)
			switch {
			case logFileNameRE.MatchString(base) && failLogWrite:
				return &faultyHandle{fileHandle: handle, writeErr: syscall.ENOSPC}, nil
			case strings.HasPrefix(base, ".errlog-probe-") && failProbeWrite:
				return &faultyHandle{fileHandle: handle, writeErr: syscall.ENOSPC}, nil
			default:
				return handle, nil
			}
		},
	})

	err := store.WriteFailure(FailureRecord{
		SchemaVersion: 1,
		TS:            time.Date(2026, 3, 17, 8, 0, 0, 0, time.UTC),
		RequestID:     "req-write-fail",
		HTTP:          HTTPInfo{Method: "GET", Path: "/sub"},
		Error: FailureError{
			Status: 400,
			App: model.AppError{
				Code:    "INVALID_ARGUMENT",
				Message: "缺少 mode 参数",
				Stage:   "validate_request",
			},
		},
	})
	if err == nil {
		t.Fatalf("WriteFailure should fail")
	}

	status := store.Health(time.Date(2026, 3, 17, 8, 0, 1, 0, time.UTC))
	if status.Healthy {
		t.Fatalf("Health()=%+v, want unhealthy", status)
	}
	if status.Reason == "" {
		t.Fatalf("unhealthy status should include reason")
	}

	failProbeWrite = false
	status = store.Health(time.Date(2026, 3, 17, 8, 0, 2, 0, time.UTC))
	if !status.Healthy {
		t.Fatalf("Health()=%+v, want healthy after probe recovery", status)
	}
}

func TestExportZIP_NoLogFilesDoesNotDegradeHealth(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	var buf bytes.Buffer
	_, err = store.ExportZIP(&buf)
	if !errors.Is(err, ErrNoLogFiles) {
		t.Fatalf("ExportZIP err=%v, want ErrNoLogFiles", err)
	}

	status := store.Health(time.Date(2026, 3, 17, 8, 0, 0, 0, time.UTC))
	if !status.Healthy {
		t.Fatalf("Health()=%+v, want healthy", status)
	}
	assertNoBusinessLogFiles(t, store.Dir())
}

func TestExportZIP_ReadFailureDegradesStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "errors-2026-03-17.jsonl")
	if err := os.WriteFile(path, []byte("{\"ok\":1}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	failProbeWrite := true
	store := newStoreWithFS(dir, faultFS{
		base: osFS{},
		openFile: func(name string, flag int, perm os.FileMode) (fileHandle, error) {
			base := filepath.Base(name)
			if logFileNameRE.MatchString(base) && flag == os.O_RDONLY {
				return nil, fs.ErrPermission
			}
			handle, err := osFS{}.OpenFile(name, flag, perm)
			if err != nil {
				return nil, err
			}
			if strings.HasPrefix(base, ".errlog-probe-") && failProbeWrite {
				return &faultyHandle{fileHandle: handle, writeErr: syscall.ENOSPC}, nil
			}
			return handle, nil
		},
	})

	var buf bytes.Buffer
	if _, err := store.ExportZIP(&buf); err == nil {
		t.Fatalf("ExportZIP should fail")
	}

	status := store.Health(time.Date(2026, 3, 17, 8, 0, 1, 0, time.UTC))
	if status.Healthy {
		t.Fatalf("Health()=%+v, want unhealthy after export failure", status)
	}

	failProbeWrite = false
	status = store.Health(time.Date(2026, 3, 17, 8, 0, 2, 0, time.UTC))
	if !status.Healthy {
		t.Fatalf("Health()=%+v, want healthy after probe recovery", status)
	}
}

func TestExportZIP_OnlyLogFiles(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	if err := os.WriteFile(filepath.Join(store.Dir(), "errors-2026-03-16.jsonl"), []byte("{\"ok\":1}\n"), 0o600); err != nil {
		t.Fatalf("write day1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir(), "errors-2026-03-17.jsonl"), []byte("{\"ok\":2}\n"), 0o600); err != nil {
		t.Fatalf("write day2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir(), "notes.txt"), []byte("ignore"), 0o600); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	var buf bytes.Buffer
	n, err := store.ExportZIP(&buf)
	if err != nil {
		t.Fatalf("ExportZIP: %v", err)
	}
	if n != 2 {
		t.Fatalf("entries=%d, want=2", n)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	if len(zr.File) != 2 {
		t.Fatalf("zip files=%d, want=2", len(zr.File))
	}
	if zr.File[0].Name != "errors-2026-03-16.jsonl" || zr.File[1].Name != "errors-2026-03-17.jsonl" {
		t.Fatalf("zip names=%q,%q", zr.File[0].Name, zr.File[1].Name)
	}
}

func assertNoBusinessLogFiles(t *testing.T, dir string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	for _, entry := range entries {
		if logFileNameRE.MatchString(entry.Name()) {
			t.Fatalf("unexpected business log file: %s", entry.Name())
		}
		if strings.HasPrefix(entry.Name(), ".errlog-probe-") {
			t.Fatalf("unexpected leftover probe file: %s", entry.Name())
		}
	}
}

type faultFS struct {
	base     fileSystem
	stat     func(name string) (fs.FileInfo, error)
	readDir  func(name string) ([]fs.DirEntry, error)
	openFile func(name string, flag int, perm os.FileMode) (fileHandle, error)
	remove   func(name string) error
}

func (f faultFS) Stat(name string) (fs.FileInfo, error) {
	if f.stat != nil {
		return f.stat(name)
	}
	return f.baseFS().Stat(name)
}

func (f faultFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if f.readDir != nil {
		return f.readDir(name)
	}
	return f.baseFS().ReadDir(name)
}

func (f faultFS) OpenFile(name string, flag int, perm os.FileMode) (fileHandle, error) {
	if f.openFile != nil {
		return f.openFile(name, flag, perm)
	}
	return f.baseFS().OpenFile(name, flag, perm)
}

func (f faultFS) Remove(name string) error {
	if f.remove != nil {
		return f.remove(name)
	}
	return f.baseFS().Remove(name)
}

func (f faultFS) baseFS() fileSystem {
	if f.base != nil {
		return f.base
	}
	return osFS{}
}

type faultyHandle struct {
	fileHandle
	writeErr   error
	syncErr    error
	closeErr   error
	shortWrite bool
}

func (h *faultyHandle) Write(p []byte) (int, error) {
	if h.writeErr != nil {
		return 0, h.writeErr
	}
	if h.shortWrite {
		if len(p) == 0 {
			return 0, nil
		}
		return len(p) - 1, nil
	}
	return h.fileHandle.Write(p)
}

func (h *faultyHandle) Sync() error {
	if h.syncErr != nil {
		return h.syncErr
	}
	return h.fileHandle.Sync()
}

func (h *faultyHandle) Close() error {
	var errs []error
	if h.fileHandle != nil {
		if err := h.fileHandle.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if h.closeErr != nil {
		errs = append(errs, h.closeErr)
	}
	return errors.Join(errs...)
}
