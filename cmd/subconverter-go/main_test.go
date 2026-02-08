package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDeriveHealthzURL_FromListenAddr(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"127.0.0.1:25500", "http://127.0.0.1:25500/healthz"},
		{"0.0.0.0:25500", "http://127.0.0.1:25500/healthz"},
		{":25500", "http://127.0.0.1:25500/healthz"},
		{"25500", "http://127.0.0.1:25500/healthz"},
		{"http://127.0.0.1:25500", "http://127.0.0.1:25500/healthz"},
	}
	for _, tt := range tests {
		got, err := deriveHealthzURL(tt.in)
		if err != nil {
			t.Fatalf("deriveHealthzURL(%q) unexpected err: %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("deriveHealthzURL(%q)=%q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRunHealthcheck_OK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	}))
	defer ts.Close()

	if err := runHealthcheck(ts.URL+"/healthz", 200*time.Millisecond); err != nil {
		t.Fatalf("runHealthcheck unexpected err: %v", err)
	}
}

func TestRunHealthcheck_StatusNotOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	err := runHealthcheck(ts.URL, 200*time.Millisecond)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "unexpected status") {
		t.Fatalf("err=%q, want contains %q", err.Error(), "unexpected status")
	}
}

