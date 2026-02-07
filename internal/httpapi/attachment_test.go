package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestFileName_ContentDispositionAndSurgeManagedURL(t *testing.T) {
	up := newMaterialsUpstream(t)
	defer up.Close()

	subURL := up.URL + "/materials/subscriptions/ss.b64"
	profileURL := up.URL + "/materials/profile.yaml"

	mux := NewMux()

	req := httptest.NewRequest(http.MethodGet, "/sub?mode=config&target=surge&fileName=my_surge&sub="+url.QueryEscape(subURL)+"&profile="+url.QueryEscape(profileURL), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Header: attachment filename (server adds extension when missing).
	cd := rr.Header().Get("Content-Disposition")
	if !strings.Contains(cd, `filename="my_surge.conf"`) {
		t.Fatalf("Content-Disposition=%q, want contains filename", cd)
	}

	// Body: managed-config URL should carry fileName for future updates.
	if !strings.Contains(rr.Body.String(), "fileName=my_surge") {
		t.Fatalf("surge body should contain fileName in managed-config URL, got:\n%s", rr.Body.String())
	}
}

func TestFileName_ListMode_ContentDisposition(t *testing.T) {
	up := newMaterialsUpstream(t)
	defer up.Close()

	subURL := up.URL + "/materials/subscriptions/ss.b64"
	mux := NewMux()

	req := httptest.NewRequest(http.MethodGet, "/sub?mode=list&encode=raw&fileName=ss&sub="+url.QueryEscape(subURL), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	cd := rr.Header().Get("Content-Disposition")
	if !strings.Contains(cd, `filename="ss.txt"`) {
		t.Fatalf("Content-Disposition=%q, want contains filename", cd)
	}
}

func TestFileName_Default_ListMode_ContentDisposition(t *testing.T) {
	up := newMaterialsUpstream(t)
	defer up.Close()

	subURL := up.URL + "/materials/subscriptions/ss.b64"
	mux := NewMux()

	req := httptest.NewRequest(http.MethodGet, "/sub?mode=list&encode=raw&sub="+url.QueryEscape(subURL), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	cd := rr.Header().Get("Content-Disposition")
	if !strings.Contains(cd, `filename="ss.txt"`) {
		t.Fatalf("Content-Disposition=%q, want contains filename", cd)
	}
}

func TestFileName_Default_ConfigMode_ContentDisposition(t *testing.T) {
	up := newMaterialsUpstream(t)
	defer up.Close()

	subURL := up.URL + "/materials/subscriptions/ss.b64"
	profileURL := up.URL + "/materials/profile.yaml"
	mux := NewMux()

	req := httptest.NewRequest(http.MethodGet, "/sub?mode=config&target=clash&sub="+url.QueryEscape(subURL)+"&profile="+url.QueryEscape(profileURL), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	cd := rr.Header().Get("Content-Disposition")
	if !strings.Contains(cd, `filename="clash.yaml"`) {
		t.Fatalf("Content-Disposition=%q, want contains filename", cd)
	}
}
