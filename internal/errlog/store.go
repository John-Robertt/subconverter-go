package errlog

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/John-Robertt/subconverter-go/internal/model"
)

const (
	schemaVersion = 1
	previewLimit  = 4 * 1024

	opWriteFailure  = "write_failure"
	opExportFailure = "export_failure"
	opDirCheck      = "dir_check"
)

var (
	logFileNameRE = regexp.MustCompile(`^errors-\d{4}-\d{2}-\d{2}\.jsonl$`)
	urlLikeRE     = regexp.MustCompile(`https?://[^\s"'<>]+`)

	ErrNoLogFiles = errors.New("no error log files")
)

type ResourceKind string

const (
	ResourceSubscription ResourceKind = "subscription"
	ResourceProfile      ResourceKind = "profile"
	ResourceTemplate     ResourceKind = "template"
)

type FailureRecord struct {
	SchemaVersion int                `json:"schema_version"`
	TS            time.Time          `json:"ts"`
	RequestID     string             `json:"request_id"`
	HTTP          HTTPInfo           `json:"http"`
	Convert       ConvertInfo        `json:"convert"`
	Error         FailureError       `json:"error"`
	Resources     []ResourceSnapshot `json:"resources,omitempty"`
	Summary       Summary            `json:"summary"`
}

type HTTPInfo struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

type ConvertInfo struct {
	Mode     string   `json:"mode,omitempty"`
	Target   string   `json:"target,omitempty"`
	Subs     []string `json:"subs,omitempty"`
	Profile  string   `json:"profile,omitempty"`
	FileName string   `json:"file_name,omitempty"`
	Encode   string   `json:"encode,omitempty"`
}

type FailureError struct {
	Status int            `json:"status"`
	App    model.AppError `json:"app"`
	Cause  string         `json:"cause,omitempty"`
}

type ResourceSnapshot struct {
	Kind             string `json:"kind"`
	URL              string `json:"url"`
	Bytes            int    `json:"bytes"`
	SHA256           string `json:"sha256"`
	Preview          string `json:"preview,omitempty"`
	PreviewTruncated bool   `json:"preview_truncated,omitempty"`
	Redacted         bool   `json:"redacted,omitempty"`
}

type Summary struct {
	SubCount           int `json:"sub_count,omitempty"`
	UniqueSubCount     int `json:"unique_sub_count,omitempty"`
	ParsedProxyCount   int `json:"parsed_proxy_count,omitempty"`
	CompiledProxyCount int `json:"compiled_proxy_count,omitempty"`
	GroupCount         int `json:"group_count,omitempty"`
	RuleCount          int `json:"rule_count,omitempty"`
}

type Collector struct {
	requestID string
	method    string
	path      string

	convert   ConvertInfo
	resources []ResourceSnapshot
	summary   Summary
}

func NewCollector(requestID string, r *http.Request) *Collector {
	c := &Collector{requestID: strings.TrimSpace(requestID)}
	if r != nil {
		c.method = r.Method
		c.path = r.URL.Path
	}
	return c
}

func (c *Collector) SetRequest(mode, target string, subs []string, profileURL, fileName, encode string) {
	if c == nil {
		return
	}
	out := make([]string, 0, len(subs))
	seen := make(map[string]struct{}, len(subs))
	for _, raw := range subs {
		raw = strings.TrimSpace(raw)
		if raw != "" {
			seen[raw] = struct{}{}
		}
		u := sanitizeURL(raw)
		if u == "" {
			continue
		}
		out = append(out, u)
	}
	c.convert = ConvertInfo{
		Mode:     strings.TrimSpace(mode),
		Target:   strings.TrimSpace(target),
		Subs:     out,
		Profile:  sanitizeURL(profileURL),
		FileName: strings.TrimSpace(fileName),
		Encode:   strings.TrimSpace(encode),
	}
	c.summary.SubCount = len(out)
	c.summary.UniqueSubCount = len(seen)
}

func (c *Collector) AddResource(snapshot ResourceSnapshot) {
	if c == nil {
		return
	}
	c.resources = append(c.resources, snapshot)
}

func (c *Collector) SetParsedProxyCount(n int) {
	if c == nil {
		return
	}
	c.summary.ParsedProxyCount = n
}

func (c *Collector) SetCompiledCounts(proxyCount, groupCount, ruleCount int) {
	if c == nil {
		return
	}
	c.summary.CompiledProxyCount = proxyCount
	c.summary.GroupCount = groupCount
	c.summary.RuleCount = ruleCount
}

func (c *Collector) BuildFailure(now time.Time, status int, app model.AppError, cause error) FailureRecord {
	if now.IsZero() {
		now = time.Now()
	}
	rec := FailureRecord{
		SchemaVersion: schemaVersion,
		TS:            now,
		HTTP: HTTPInfo{
			Method: c.method,
			Path:   c.path,
		},
		Convert:   c.convert,
		Resources: append([]ResourceSnapshot(nil), c.resources...),
		Summary:   c.summary,
		Error: FailureError{
			Status: status,
			App:    sanitizeAppError(app),
			Cause:  sanitizeTextURLs(errorString(cause)),
		},
	}
	rec.RequestID = c.requestID
	return rec
}

func NewResourceSnapshot(kind ResourceKind, rawURL string, content string) ResourceSnapshot {
	snapshot := ResourceSnapshot{
		Kind: string(kind),
		URL:  sanitizeURL(rawURL),
	}

	sum := sha256.Sum256([]byte(content))
	snapshot.SHA256 = hex.EncodeToString(sum[:])
	snapshot.Bytes = len([]byte(content))

	switch kind {
	case ResourceSubscription:
		snapshot.Redacted = true
	default:
		previewRaw, truncated := truncateUTF8(content, previewLimit)
		preview := sanitizeTextURLs(previewRaw)
		snapshot.Preview = preview
		snapshot.PreviewTruncated = truncated
		snapshot.Redacted = snapshot.URL != strings.TrimSpace(rawURL) || preview != previewRaw
	}

	return snapshot
}

type HealthStatus struct {
	Healthy bool
	Reason  string
	Since   time.Time
}

type fileHandle interface {
	io.Reader
	io.Writer
	Sync() error
	Close() error
}

type fileSystem interface {
	Stat(name string) (fs.FileInfo, error)
	ReadDir(name string) ([]fs.DirEntry, error)
	OpenFile(name string, flag int, perm os.FileMode) (fileHandle, error)
	Remove(name string) error
}

type osFS struct{}

func (osFS) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (osFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}

func (osFS) OpenFile(name string, flag int, perm os.FileMode) (fileHandle, error) {
	return os.OpenFile(name, flag, perm)
}

func (osFS) Remove(name string) error {
	return os.Remove(name)
}

type degradedState struct {
	op     string
	reason string
	since  time.Time
}

type Store struct {
	dir string
	fs  fileSystem

	ioMu    sync.Mutex
	stateMu sync.RWMutex

	degradedSince  time.Time
	degradedReason string
	degradedOp     string
}

func NewStore(dir string) (*Store, error) {
	resolved := strings.TrimSpace(dir)
	if resolved == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		resolved = wd
	}
	resolved = filepath.Clean(resolved)
	if err := os.MkdirAll(resolved, 0o755); err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("error-log-dir 不是目录")
	}
	return newStoreWithFS(resolved, osFS{}), nil
}

func newStoreWithFS(dir string, fsys fileSystem) *Store {
	if fsys == nil {
		fsys = osFS{}
	}
	return &Store{
		dir: filepath.Clean(strings.TrimSpace(dir)),
		fs:  fsys,
	}
}

func (s *Store) Dir() string {
	if s == nil {
		return ""
	}
	return s.dir
}

func (s *Store) ValidateStartup(now time.Time) error {
	if s == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	if err := s.validateDirStructure(); err != nil {
		return err
	}
	if err := s.runProbe(now); err != nil {
		return err
	}
	s.clearDegraded()
	return nil
}

func (s *Store) Health(now time.Time) HealthStatus {
	if s == nil {
		return HealthStatus{Healthy: true}
	}
	if now.IsZero() {
		now = time.Now()
	}

	if err := s.validateDirStructure(); err != nil {
		s.markDegraded(opDirCheck, err)
		return s.unhealthyStatus()
	}

	if !s.hasDegraded() {
		return HealthStatus{Healthy: true}
	}

	if err := s.runProbe(now); err != nil {
		s.markDegraded(opWriteFailure, err)
		return s.unhealthyStatus()
	}

	s.clearDegraded()
	return HealthStatus{Healthy: true}
}

func (s *Store) WriteFailure(record FailureRecord) error {
	if s == nil {
		return nil
	}

	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	s.ioMu.Lock()
	defer s.ioMu.Unlock()

	f, err := s.fs.OpenFile(s.filePath(record.TS), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		s.markDegraded(opWriteFailure, err)
		return err
	}

	var errs []error
	if n, err := f.Write(data); err != nil {
		errs = append(errs, err)
	} else if n != len(data) {
		errs = append(errs, io.ErrShortWrite)
	}
	if err := f.Sync(); err != nil {
		errs = append(errs, err)
	}
	if err := f.Close(); err != nil {
		errs = append(errs, err)
	}

	if err := errors.Join(errs...); err != nil {
		s.markDegraded(opWriteFailure, err)
		return err
	}

	s.clearDegradedIf(opWriteFailure)
	return nil
}

func (s *Store) ExportZIP(w io.Writer) (int, error) {
	if s == nil {
		return 0, ErrNoLogFiles
	}

	s.ioMu.Lock()
	defer s.ioMu.Unlock()

	names, err := s.listLogFiles()
	if err != nil {
		s.markDegraded(opExportFailure, err)
		return 0, err
	}
	if len(names) == 0 {
		return 0, ErrNoLogFiles
	}

	zw := zip.NewWriter(w)
	for _, name := range names {
		path := filepath.Join(s.dir, name)
		info, err := s.fs.Stat(path)
		if err != nil {
			_ = zw.Close()
			s.markDegraded(opExportFailure, err)
			return 0, err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			_ = zw.Close()
			return 0, err
		}
		h.Name = name
		h.Method = zip.Deflate

		dst, err := zw.CreateHeader(h)
		if err != nil {
			_ = zw.Close()
			return 0, err
		}

		src, err := s.fs.OpenFile(path, os.O_RDONLY, 0)
		if err != nil {
			_ = zw.Close()
			s.markDegraded(opExportFailure, err)
			return 0, err
		}

		_, copyErr := io.Copy(dst, src)
		closeErr := src.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			_ = zw.Close()
			s.markDegraded(opExportFailure, err)
			return 0, err
		}
	}
	if err := zw.Close(); err != nil {
		return 0, err
	}

	s.clearDegradedIf(opExportFailure)
	return len(names), nil
}

func (s *Store) validateDirStructure() error {
	info, err := s.fs.Stat(s.dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("error-log-dir 不是目录")
	}
	if _, err := s.fs.ReadDir(s.dir); err != nil {
		return err
	}
	return nil
}

func (s *Store) runProbe(now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}

	probePath := s.probePath(now)
	s.ioMu.Lock()
	defer s.ioMu.Unlock()

	f, err := s.fs.OpenFile(probePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}

	var errs []error
	if n, err := f.Write([]byte("1")); err != nil {
		errs = append(errs, err)
	} else if n != 1 {
		errs = append(errs, io.ErrShortWrite)
	}
	if err := f.Sync(); err != nil {
		errs = append(errs, err)
	}
	if err := f.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := s.fs.Remove(probePath); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (s *Store) filePath(now time.Time) string {
	return filepath.Join(s.dir, "errors-"+now.Format("2006-01-02")+".jsonl")
}

func (s *Store) probePath(now time.Time) string {
	return filepath.Join(s.dir, fmt.Sprintf(".errlog-probe-%d-%d", os.Getpid(), now.UnixNano()))
}

func (s *Store) listLogFiles() ([]string, error) {
	entries, err := s.fs.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !logFileNameRE.MatchString(name) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (s *Store) markDegraded(op string, err error) {
	if s == nil || err == nil {
		return
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	now := time.Now()
	if s.degradedSince.IsZero() || s.degradedOp != op {
		s.degradedSince = now
	}
	s.degradedOp = op
	s.degradedReason = err.Error()
}

func (s *Store) clearDegradedIf(op string) {
	if s == nil {
		return
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.degradedOp != op {
		return
	}
	s.clearDegradedLocked()
}

func (s *Store) clearDegraded() {
	if s == nil {
		return
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.clearDegradedLocked()
}

func (s *Store) clearDegradedLocked() {
	s.degradedSince = time.Time{}
	s.degradedReason = ""
	s.degradedOp = ""
}

func (s *Store) hasDegraded() bool {
	if s == nil {
		return false
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return !s.degradedSince.IsZero()
}

func (s *Store) unhealthyStatus() HealthStatus {
	if s == nil {
		return HealthStatus{Healthy: true}
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return HealthStatus{
		Healthy: false,
		Reason:  s.degradedReason,
		Since:   s.degradedSince,
	}
}

func sanitizeAppError(app model.AppError) model.AppError {
	app.URL = sanitizeURL(app.URL)
	app.Hint = sanitizeTextURLs(app.Hint)
	return app
}

func sanitizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return raw
	}
	if u.RawQuery == "" {
		u.Fragment = ""
		return u.String()
	}

	keys := make([]string, 0)
	seen := make(map[string]struct{})
	for _, part := range strings.Split(u.RawQuery, "&") {
		if part == "" {
			continue
		}
		key, _, _ := strings.Cut(part, "=")
		key, err = url.QueryUnescape(key)
		if err != nil {
			key = strings.TrimSpace(key)
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		u.RawQuery = ""
		u.Fragment = ""
		return u.String()
	}

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, url.QueryEscape(key)+"=%3Credacted%3E")
	}
	u.RawQuery = strings.Join(parts, "&")
	u.Fragment = ""
	return u.String()
}

func sanitizeTextURLs(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	return urlLikeRE.ReplaceAllStringFunc(s, sanitizeURL)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func truncateUTF8(s string, limit int) (string, bool) {
	if limit <= 0 || len([]byte(s)) <= limit {
		return s, false
	}
	b := []byte(s)
	b = b[:limit]
	for len(b) > 0 && !utf8.Valid(b) {
		b = b[:len(b)-1]
	}
	return string(b), true
}
