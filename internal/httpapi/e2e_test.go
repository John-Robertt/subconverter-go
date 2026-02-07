package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestE2E_Materials_ConfigAndList(t *testing.T) {
	// Serve docs/materials via an upstream server.
	up := newMaterialsUpstream(t)
	defer up.Close()

	subURL := up.URL + "/materials/subscriptions/ss.b64"
	profileURL := up.URL + "/materials/profile.yaml"

	mux := NewMux()

	// mode=config
	{
		wantClash := expectedClashConfig(profileURL)
		gotClash := doGET(t, mux, "/sub?mode=config&target=clash&sub="+url.QueryEscape(subURL)+"&profile="+url.QueryEscape(profileURL))
		if gotClash != wantClash {
			i := firstDiff(gotClash, wantClash)
			t.Fatalf("clash output mismatch (len got=%d want=%d firstDiff=%d)\n--- got ---\n%s\n--- want ---\n%s", len(gotClash), len(wantClash), i, gotClash, wantClash)
		}
		gotClashPOST := doPOSTJSON(t, mux, "/api/convert", map[string]any{
			"mode":    "config",
			"target":  "clash",
			"subs":    []string{subURL},
			"profile": profileURL,
		})
		if gotClashPOST != gotClash {
			t.Fatalf("clash GET/POST mismatch\n--- GET ---\n%s\n--- POST ---\n%s", gotClash, gotClashPOST)
		}
	}

	{
		wantSR := expectedShadowrocketConfig(profileURL)
		gotSR := doGET(t, mux, "/sub?mode=config&target=shadowrocket&sub="+url.QueryEscape(subURL)+"&profile="+url.QueryEscape(profileURL))
		if gotSR != wantSR {
			t.Fatalf("shadowrocket output mismatch\n--- got ---\n%s\n--- want ---\n%s", gotSR, wantSR)
		}
		gotSRPOST := doPOSTJSON(t, mux, "/api/convert", map[string]any{
			"mode":    "config",
			"target":  "shadowrocket",
			"subs":    []string{subURL},
			"profile": profileURL,
		})
		if gotSRPOST != gotSR {
			t.Fatalf("shadowrocket GET/POST mismatch\n--- GET ---\n%s\n--- POST ---\n%s", gotSR, gotSRPOST)
		}
	}

	{
		wantSurge := expectedSurgeConfig(subURL, profileURL)
		gotSurge := doGET(t, mux, "/sub?mode=config&target=surge&sub="+url.QueryEscape(subURL)+"&profile="+url.QueryEscape(profileURL))
		if gotSurge != wantSurge {
			t.Fatalf("surge output mismatch\n--- got ---\n%s\n--- want ---\n%s", gotSurge, wantSurge)
		}
		gotSurgePOST := doPOSTJSON(t, mux, "/api/convert", map[string]any{
			"mode":    "config",
			"target":  "surge",
			"subs":    []string{subURL},
			"profile": profileURL,
		})
		if gotSurgePOST != gotSurge {
			t.Fatalf("surge GET/POST mismatch\n--- GET ---\n%s\n--- POST ---\n%s", gotSurge, gotSurgePOST)
		}
	}

	{
		wantQX := expectedQuanxConfig(profileURL)
		gotQX := doGET(t, mux, "/sub?mode=config&target=quanx&sub="+url.QueryEscape(subURL)+"&profile="+url.QueryEscape(profileURL))
		if gotQX != wantQX {
			i := firstDiff(gotQX, wantQX)
			t.Fatalf("quanx output mismatch (len got=%d want=%d firstDiff=%d)\n--- got ---\n%s\n--- want ---\n%s", len(gotQX), len(wantQX), i, gotQX, wantQX)
		}
		gotQXPOST := doPOSTJSON(t, mux, "/api/convert", map[string]any{
			"mode":    "config",
			"target":  "quanx",
			"subs":    []string{subURL},
			"profile": profileURL,
		})
		if gotQXPOST != gotQX {
			t.Fatalf("quanx GET/POST mismatch\n--- GET ---\n%s\n--- POST ---\n%s", gotQX, gotQXPOST)
		}
	}

	// mode=list
	{
		wantRaw := expectedListRaw()
		gotRaw := doGET(t, mux, "/sub?mode=list&encode=raw&sub="+url.QueryEscape(subURL))
		if gotRaw != wantRaw {
			t.Fatalf("list raw mismatch\n--- got ---\n%q\n--- want ---\n%q", gotRaw, wantRaw)
		}

		wantB64 := base64.StdEncoding.EncodeToString([]byte(wantRaw))
		gotB64 := doGET(t, mux, "/sub?mode=list&sub="+url.QueryEscape(subURL)) // default base64
		if gotB64 != wantB64 {
			t.Fatalf("list base64 mismatch\n--- got ---\n%q\n--- want ---\n%q", gotB64, wantB64)
		}

		gotRawPOST := doPOSTJSON(t, mux, "/api/convert", map[string]any{
			"mode":   "list",
			"subs":   []string{subURL},
			"encode": "raw",
		})
		if gotRawPOST != wantRaw {
			t.Fatalf("list raw POST mismatch\n--- got ---\n%q\n--- want ---\n%q", gotRawPOST, wantRaw)
		}
	}
}

func doGET(t *testing.T, mux http.Handler, path string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s status=%d body=%s", path, rr.Code, rr.Body.String())
	}
	if got, want := rr.Header().Get("Content-Type"), "text/plain; charset=utf-8"; got != want {
		t.Fatalf("GET %s Content-Type=%q, want=%q", path, got, want)
	}
	return rr.Body.String()
}

func doPOSTJSON(t *testing.T, mux http.Handler, path string, payload any) string {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST %s status=%d body=%s", path, rr.Code, rr.Body.String())
	}
	if got, want := rr.Header().Get("Content-Type"), "text/plain; charset=utf-8"; got != want {
		t.Fatalf("POST %s Content-Type=%q, want=%q", path, got, want)
	}
	return rr.Body.String()
}

func newMaterialsUpstream(t *testing.T) *httptest.Server {
	t.Helper()

	root := repoRoot(t)
	read := func(rel string) []byte {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return b
	}

	subB64 := read("docs/materials/subscriptions/ss.b64")
	clashTmpl := read("docs/materials/templates/clash.yaml")
	surgeTmpl := read("docs/materials/templates/surge.conf")
	srTmpl := read("docs/materials/templates/shadowrocket.conf")
	quanxTmpl := read("docs/materials/templates/quanx.conf")
	lan := read("docs/materials/rulesets/LAN.list")
	banad := read("docs/materials/rulesets/BanAD.list")
	proxy := read("docs/materials/rulesets/Proxy.list")

	mux := http.NewServeMux()
	mux.HandleFunc("/materials/subscriptions/ss.b64", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(subB64)
	})
	mux.HandleFunc("/materials/templates/clash.yaml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(clashTmpl)
	})
	mux.HandleFunc("/materials/templates/surge.conf", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(surgeTmpl)
	})
	mux.HandleFunc("/materials/templates/shadowrocket.conf", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(srTmpl)
	})
	mux.HandleFunc("/materials/templates/quanx.conf", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(quanxTmpl)
	})
	mux.HandleFunc("/materials/rulesets/LAN.list", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(lan)
	})
	mux.HandleFunc("/materials/rulesets/BanAD.list", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(banad)
	})
	mux.HandleFunc("/materials/rulesets/Proxy.list", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(proxy)
	})
	mux.HandleFunc("/materials/profile.yaml", func(w http.ResponseWriter, r *http.Request) {
		// Generate URLs pointing back to this upstream server.
		base := "http://" + r.Host
		profileYAML := "" +
			"version: 1\n\n" +
			"template:\n" +
			"  clash: \"" + base + "/materials/templates/clash.yaml\"\n" +
			"  shadowrocket: \"" + base + "/materials/templates/shadowrocket.conf\"\n" +
			"  surge: \"" + base + "/materials/templates/surge.conf\"\n" +
			"  quanx: \"" + base + "/materials/templates/quanx.conf\"\n\n" +
			"public_base_url: \"https://public.example.com/sub\"\n\n" +
			"custom_proxy_group:\n" +
			"  - \"PROXY`select`[]AUTO[]@all[]DIRECT\"\n" +
			"  - \"AUTO`url-test`(Example|HK|SG|US)`http://www.gstatic.com/generate_204`300`50\"\n\n" +
			"ruleset:\n" +
			"  - \"DIRECT," + base + "/materials/rulesets/LAN.list\"\n" +
			"  - \"REJECT," + base + "/materials/rulesets/BanAD.list\"\n" +
			"  - \"PROXY," + base + "/materials/rulesets/Proxy.list\"\n\n" +
			"rule:\n" +
			"  - \"MATCH,PROXY\"\n"
		_, _ = w.Write([]byte(profileYAML))
	})

	return httptest.NewServer(mux)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	// file = .../internal/httpapi/e2e_test.go
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func expectedClashConfig(profileURL string) string {
	base := materialsBase(profileURL)
	lan := base + "/materials/rulesets/LAN.list"
	banad := base + "/materials/rulesets/BanAD.list"
	proxy := base + "/materials/rulesets/Proxy.list"
	return "" +
		"mixed-port: 7890\n" +
		"allow-lan: false\n" +
		"mode: rule\n" +
		"log-level: info\n" +
		"\n" +
		"proxies:\n" +
		"  - name: \"Example-HK\"\n" +
		"    type: ss\n" +
		"    server: \"hk.example.com\"\n" +
		"    port: 8388\n" +
		"    cipher: \"aes-128-gcm\"\n" +
		"    password: \"password\"\n" +
		"  - name: \"Example-SG\"\n" +
		"    type: ss\n" +
		"    server: \"sg.example.com\"\n" +
		"    port: 8388\n" +
		"    cipher: \"chacha20-ietf-poly1305\"\n" +
		"    password: \"pass123\"\n" +
		"  - name: \"Example-US\"\n" +
		"    type: ss\n" +
		"    server: \"us.example.com\"\n" +
		"    port: 8388\n" +
		"    cipher: \"aes-256-gcm\"\n" +
		"    password: \"passw0rd\"\n" +
		"\n" +
		"proxy-groups:\n" +
		"  - name: \"PROXY\"\n" +
		"    type: \"select\"\n" +
		"    proxies:\n" +
		"      - \"AUTO\"\n" +
		"      - \"Example-HK\"\n" +
		"      - \"Example-SG\"\n" +
		"      - \"Example-US\"\n" +
		"      - \"DIRECT\"\n" +
		"  - name: \"AUTO\"\n" +
		"    type: \"url-test\"\n" +
		"    proxies:\n" +
		"      - \"Example-HK\"\n" +
		"      - \"Example-SG\"\n" +
		"      - \"Example-US\"\n" +
		"    url: \"http://www.gstatic.com/generate_204\"\n" +
		"    interval: 300\n" +
		"    tolerance: 50\n" +
		"\n" +
		"rule-providers:\n" +
		"  LAN:\n" +
		"    type: http\n" +
		"    behavior: classical\n" +
		"    url: \"" + lan + "\"\n" +
		"    interval: 86400\n" +
		"    format: text\n" +
		"  BanAD:\n" +
		"    type: http\n" +
		"    behavior: classical\n" +
		"    url: \"" + banad + "\"\n" +
		"    interval: 86400\n" +
		"    format: text\n" +
		"  Proxy:\n" +
		"    type: http\n" +
		"    behavior: classical\n" +
		"    url: \"" + proxy + "\"\n" +
		"    interval: 86400\n" +
		"    format: text\n" +
		"\n" +
		"rules:\n" +
		"  - \"RULE-SET,LAN,DIRECT\"\n" +
		"  - \"RULE-SET,BanAD,REJECT\"\n" +
		"  - \"RULE-SET,Proxy,PROXY\"\n" +
		"  - \"MATCH,PROXY\"\n"
}

func expectedShadowrocketConfig(profileURL string) string {
	base := materialsBase(profileURL)
	lan := base + "/materials/rulesets/LAN.list"
	banad := base + "/materials/rulesets/BanAD.list"
	proxy := base + "/materials/rulesets/Proxy.list"
	return "" +
		"[General]\n" +
		"loglevel = notify\n" +
		"\n" +
		"[Proxy]\n" +
		"DIRECT = direct\n" +
		"REJECT = reject\n" +
		"Example-HK = ss, hk.example.com, 8388, encrypt-method=aes-128-gcm, password=password\n" +
		"Example-SG = ss, sg.example.com, 8388, encrypt-method=chacha20-ietf-poly1305, password=pass123\n" +
		"Example-US = ss, us.example.com, 8388, encrypt-method=aes-256-gcm, password=passw0rd\n" +
		"\n" +
		"[Proxy Group]\n" +
		"PROXY = select, AUTO, Example-HK, Example-SG, Example-US, DIRECT\n" +
		"AUTO = url-test, Example-HK, Example-SG, Example-US, url=http://www.gstatic.com/generate_204, interval=300, tolerance=50\n" +
		"\n" +
		"[Rule]\n" +
		"RULE-SET," + lan + ",DIRECT\n" +
		"RULE-SET," + banad + ",REJECT\n" +
		"RULE-SET," + proxy + ",PROXY\n" +
		"FINAL,PROXY\n" +
		"\n"
}

func expectedSurgeConfig(subURL, profileURL string) string {
	managedURL := "https://public.example.com/sub?mode=config&target=surge&sub=" + pctEncodeExpect(subURL) + "&profile=" + pctEncodeExpect(profileURL)
	return "" +
		"#!MANAGED-CONFIG " + managedURL + " interval=86400\n" +
		expectedSurgeLikeBody(profileURL)
}

func expectedSurgeLikeBody(profileURL string) string {
	base := materialsBase(profileURL)
	lan := base + "/materials/rulesets/LAN.list"
	banad := base + "/materials/rulesets/BanAD.list"
	proxy := base + "/materials/rulesets/Proxy.list"
	// Surge 不允许在 [Proxy] 段定义内部策略名（DIRECT/REJECT）。
	return "" +
		"[General]\n" +
		"loglevel = notify\n" +
		"\n" +
		"[Proxy]\n" +
		"Example-HK = ss, hk.example.com, 8388, encrypt-method=aes-128-gcm, password=password\n" +
		"Example-SG = ss, sg.example.com, 8388, encrypt-method=chacha20-ietf-poly1305, password=pass123\n" +
		"Example-US = ss, us.example.com, 8388, encrypt-method=aes-256-gcm, password=passw0rd\n" +
		"\n" +
		"[Proxy Group]\n" +
		"PROXY = select, AUTO, Example-HK, Example-SG, Example-US, DIRECT\n" +
		"AUTO = url-test, Example-HK, Example-SG, Example-US, url=http://www.gstatic.com/generate_204, interval=300, tolerance=50\n" +
		"\n" +
		"[Rule]\n" +
		"RULE-SET," + lan + ",DIRECT\n" +
		"RULE-SET," + banad + ",REJECT\n" +
		"RULE-SET," + proxy + ",PROXY\n" +
		"FINAL,PROXY\n"
}

func expectedQuanxConfig(profileURL string) string {
	base := materialsBase(profileURL)
	lan := base + "/materials/rulesets/LAN.list"
	banad := base + "/materials/rulesets/BanAD.list"
	proxy := base + "/materials/rulesets/Proxy.list"
	return "" +
		"[general]\n" +
		"# Minimal Quantumult X template for subconverter-go v1.\n" +
		"# Blocks are injected by anchors (do not write anchor tokens in comments, or template validation will fail):\n" +
		"# - PROXIES anchor into [server_local]\n" +
		"# - GROUPS anchor into [policy]\n" +
		"# - RULESETS anchor into [filter_remote]\n" +
		"# - RULES anchor into [filter_local]\n" +
		"\n" +
		"network_check_url=http://www.gstatic.com/generate_204\n" +
		"server_check_url=http://www.gstatic.com/generate_204\n" +
		"\n" +
		"[dns]\n" +
		"server=1.1.1.1\n" +
		"server=8.8.8.8\n" +
		"\n" +
		"[policy]\n" +
		"static=PROXY, AUTO, Example-HK, Example-SG, Example-US, direct\n" +
		"url-latency-benchmark=AUTO, Example-HK, Example-SG, Example-US, check-interval=300, tolerance=50\n" +
		"\n" +
		"[server_local]\n" +
		"shadowsocks = hk.example.com:8388, method=aes-128-gcm, password=password, tag=Example-HK\n" +
		"shadowsocks = sg.example.com:8388, method=chacha20-ietf-poly1305, password=pass123, tag=Example-SG\n" +
		"shadowsocks = us.example.com:8388, method=aes-256-gcm, password=passw0rd, tag=Example-US\n" +
		"\n" +
		"[filter_remote]\n" +
		lan + ", tag=DIRECT, force-policy=direct, enabled=true\n" +
		banad + ", tag=REJECT, force-policy=reject, enabled=true\n" +
		proxy + ", tag=PROXY, force-policy=PROXY, enabled=true\n" +
		"\n" +
		"[filter_local]\n" +
		"FINAL,PROXY\n"
}

func materialsBase(profileURL string) string {
	const suffix = "/materials/profile.yaml"
	if !strings.HasSuffix(profileURL, suffix) {
		// Panic in tests: this is a programmer error.
		panic("unexpected profileURL: " + profileURL)
	}
	return strings.TrimSuffix(profileURL, suffix)
}

func expectedListRaw() string {
	return "" +
		"ss://YWVzLTEyOC1nY206cGFzc3dvcmQ@hk.example.com:8388#Example-HK\n" +
		"ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYXNzMTIz@sg.example.com:8388#Example-SG\n" +
		"ss://YWVzLTI1Ni1nY206cGFzc3cwcmQ@us.example.com:8388#Example-US\n"
}

func pctEncodeExpect(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func firstDiff(a, b string) int {
	na, nb := len(a), len(b)
	n := na
	if nb < n {
		n = nb
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if na != nb {
		return n
	}
	return -1
}
