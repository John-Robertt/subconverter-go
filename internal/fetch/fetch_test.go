package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchText_UnsupportedScheme(t *testing.T) {
	_, err := FetchText(context.Background(), KindSubscription, "file:///etc/passwd")
	var fe *FetchError
	if !errors.As(err, &fe) {
		t.Fatalf("expected *FetchError, got %T: %v", err, err)
	}
	if fe.Status != http.StatusBadRequest {
		t.Fatalf("status=%d, want=%d", fe.Status, http.StatusBadRequest)
	}
	if fe.AppError.Code != "INVALID_ARGUMENT" {
		t.Fatalf("code=%q, want=%q", fe.AppError.Code, "INVALID_ARGUMENT")
	}
	if fe.AppError.Stage != "fetch_sub" {
		t.Fatalf("stage=%q, want=%q", fe.AppError.Stage, "fetch_sub")
	}
}

func TestFetchText_TooLarge(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", 32)))
	}))
	defer ts.Close()

	_, err := FetchTextWithOptions(context.Background(), KindTemplate, ts.URL, Options{MaxBytes: 10})
	var fe *FetchError
	if !errors.As(err, &fe) {
		t.Fatalf("expected *FetchError, got %T: %v", err, err)
	}
	if fe.Status != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d, want=%d", fe.Status, http.StatusUnprocessableEntity)
	}
	if fe.AppError.Code != "TOO_LARGE" {
		t.Fatalf("code=%q, want=%q", fe.AppError.Code, "TOO_LARGE")
	}
	if fe.AppError.Stage != "fetch_template" {
		t.Fatalf("stage=%q, want=%q", fe.AppError.Stage, "fetch_template")
	}
}

func TestFetchText_InvalidUTF8(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 0xff is always invalid in UTF-8.
		_, _ = w.Write([]byte{0xff, 0xfe, 0xfd})
	}))
	defer ts.Close()

	_, err := FetchText(context.Background(), KindTemplate, ts.URL)
	var fe *FetchError
	if !errors.As(err, &fe) {
		t.Fatalf("expected *FetchError, got %T: %v", err, err)
	}
	if fe.Status != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d, want=%d", fe.Status, http.StatusUnprocessableEntity)
	}
	if fe.AppError.Code != "FETCH_INVALID_UTF8" {
		t.Fatalf("code=%q, want=%q", fe.AppError.Code, "FETCH_INVALID_UTF8")
	}
	if fe.AppError.Stage != "fetch_template" {
		t.Fatalf("stage=%q, want=%q", fe.AppError.Stage, "fetch_template")
	}
}

func TestFetchText_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	_, err := FetchTextWithOptions(context.Background(), KindProfile, ts.URL, Options{Timeout: 50 * time.Millisecond})
	var fe *FetchError
	if !errors.As(err, &fe) {
		t.Fatalf("expected *FetchError, got %T: %v", err, err)
	}
	if fe.Status != http.StatusGatewayTimeout {
		t.Fatalf("status=%d, want=%d", fe.Status, http.StatusGatewayTimeout)
	}
	if fe.AppError.Code != "FETCH_TIMEOUT" {
		t.Fatalf("code=%q, want=%q", fe.AppError.Code, "FETCH_TIMEOUT")
	}
	if fe.AppError.Stage != "fetch_profile" {
		t.Fatalf("stage=%q, want=%q", fe.AppError.Stage, "fetch_profile")
	}
}

func TestFetchText_TooManyRedirects(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, ts.URL, http.StatusFound)
	}))
	defer ts.Close()

	_, err := FetchTextWithOptions(context.Background(), KindSubscription, ts.URL, Options{MaxRedirects: 2})
	var fe *FetchError
	if !errors.As(err, &fe) {
		t.Fatalf("expected *FetchError, got %T: %v", err, err)
	}
	if fe.Status != http.StatusBadGateway {
		t.Fatalf("status=%d, want=%d", fe.Status, http.StatusBadGateway)
	}
	if fe.AppError.Code != "FETCH_FAILED" {
		t.Fatalf("code=%q, want=%q", fe.AppError.Code, "FETCH_FAILED")
	}
}

func TestFetchText_RedirectToNonHTTP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	}))
	defer ts.Close()

	_, err := FetchTextWithOptions(context.Background(), KindSubscription, ts.URL, Options{MaxRedirects: 5})
	var fe *FetchError
	if !errors.As(err, &fe) {
		t.Fatalf("expected *FetchError, got %T: %v", err, err)
	}
	if fe.Status != http.StatusBadRequest {
		t.Fatalf("status=%d, want=%d", fe.Status, http.StatusBadRequest)
	}
	if fe.AppError.Code != "INVALID_ARGUMENT" {
		t.Fatalf("code=%q, want=%q", fe.AppError.Code, "INVALID_ARGUMENT")
	}
}
