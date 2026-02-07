package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchAndParseSubs_DedupURL(t *testing.T) {
	var hits atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = fmt.Fprint(w, "ss://YWVzLTEyOC1nY206cGFzc3dvcmQ=@example.com:8388#A\n")
	}))
	defer ts.Close()

	url := ts.URL
	got, err := fetchAndParseSubs(context.Background(), []string{url, url, url})
	if err != nil {
		t.Fatalf("fetchAndParseSubs error: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits=%d, want=1", hits.Load())
	}
	if len(got) != 1 {
		t.Fatalf("proxies=%d, want=1", len(got))
	}
}

func TestFetchAndParseSubs_ConcurrentFetch(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})

	subBody := "ss://YWVzLTEyOC1nY206cGFzc3dvcmQ=@example.com:8388#A\n"

	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		started <- "/a"
		<-release
		_, _ = fmt.Fprint(w, subBody)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		started <- "/b"
		<-release
		_, _ = fmt.Fprint(w, subBody)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	done := make(chan error, 1)
	go func() {
		_, err := fetchAndParseSubs(context.Background(), []string{ts.URL + "/a", ts.URL + "/b"})
		done <- err
	}()

	seen := make(map[string]bool, 2)
	for i := 0; i < 2; i++ {
		select {
		case p := <-started:
			seen[p] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for concurrent fetch start (seen=%v)", seen)
		}
	}
	if !seen["/a"] || !seen["/b"] {
		t.Fatalf("seen=%v, want both /a and /b", seen)
	}

	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("fetchAndParseSubs error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for fetchAndParseSubs to finish")
	}
}
