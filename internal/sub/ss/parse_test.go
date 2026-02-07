package ss

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/John-Robertt/subconverter-go/internal/model"
)

func TestParseSubscriptionText_RawList(t *testing.T) {
	raw := strings.Join([]string{
		"# comment",
		"  ",
		"ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#Node%201",
		"ss://YWVzLTEyOC1nY206cDI=@example.com:8389#Node%202",
		"",
	}, "\n")

	proxies, err := ParseSubscriptionText("https://example.com/sub.txt", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proxies) != 2 {
		t.Fatalf("len=%d, want=2", len(proxies))
	}
	if proxies[0].Type != "ss" {
		t.Fatalf("type=%q, want=%q", proxies[0].Type, "ss")
	}
	if proxies[0].Name != "Node 1" {
		t.Fatalf("name=%q, want=%q", proxies[0].Name, "Node 1")
	}
	if proxies[0].Server != "example.com" || proxies[0].Port != 8388 {
		t.Fatalf("server/port=%q/%d, want example.com/8388", proxies[0].Server, proxies[0].Port)
	}
}

func TestParseSubscriptionText_Base64List(t *testing.T) {
	raw := "ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#Node%201\n"
	b64 := base64.StdEncoding.EncodeToString([]byte(raw))

	proxies, err := ParseSubscriptionText("https://example.com/sub.b64", b64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proxies) != 1 {
		t.Fatalf("len=%d, want=1", len(proxies))
	}
	if proxies[0].Name != "Node 1" {
		t.Fatalf("name=%q, want=%q", proxies[0].Name, "Node 1")
	}
}

func TestParseSubscriptionText_SIP002_Plugin(t *testing.T) {
	raw := "ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388/?plugin=simple-obfs%3Bobfs%3Dtls%3Bobfs-host%3Dexample.com#obfs\n"
	proxies, err := ParseSubscriptionText("https://example.com/sub.txt", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proxies) != 1 {
		t.Fatalf("len=%d, want=1", len(proxies))
	}
	if proxies[0].PluginName != "simple-obfs" {
		t.Fatalf("plugin=%q, want=%q", proxies[0].PluginName, "simple-obfs")
	}
	if len(proxies[0].PluginOpts) != 2 {
		t.Fatalf("opts len=%d, want=2", len(proxies[0].PluginOpts))
	}
	if proxies[0].PluginOpts[0] != (model.KV{Key: "obfs", Value: "tls"}) {
		t.Fatalf("opt0=%+v, want obfs=tls", proxies[0].PluginOpts[0])
	}
	if proxies[0].PluginOpts[1] != (model.KV{Key: "obfs-host", Value: "example.com"}) {
		t.Fatalf("opt1=%+v, want obfs-host=example.com", proxies[0].PluginOpts[1])
	}
}

func TestParseSubscriptionText_OldBase64Form(t *testing.T) {
	decoded := "aes-128-gcm:pass@ex.com:443"
	b64 := base64.StdEncoding.EncodeToString([]byte(decoded))
	raw := "ss://" + b64 + "#old\n"

	proxies, err := ParseSubscriptionText("https://example.com/sub.txt", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proxies) != 1 {
		t.Fatalf("len=%d, want=1", len(proxies))
	}
	if proxies[0].Cipher != "aes-128-gcm" || proxies[0].Password != "pass" {
		t.Fatalf("cipher/password=%q/%q, want aes-128-gcm/pass", proxies[0].Cipher, proxies[0].Password)
	}
	if proxies[0].Server != "ex.com" || proxies[0].Port != 443 {
		t.Fatalf("server/port=%q/%d, want ex.com/443", proxies[0].Server, proxies[0].Port)
	}
}

func TestParseSubscriptionText_UnknownQueryParam_Strict(t *testing.T) {
	raw := "ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388/?foo=bar#x\n"
	_, err := ParseSubscriptionText("https://example.com/sub.txt", raw)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T: %v", err, err)
	}
	if pe.AppError.Code != "SUB_PARSE_ERROR" {
		t.Fatalf("code=%q, want=%q", pe.AppError.Code, "SUB_PARSE_ERROR")
	}
	if pe.AppError.Stage != "parse_sub" {
		t.Fatalf("stage=%q, want=%q", pe.AppError.Stage, "parse_sub")
	}
	if pe.AppError.Line != 1 {
		t.Fatalf("line=%d, want=1", pe.AppError.Line)
	}
	if pe.AppError.URL != "https://example.com/sub.txt" {
		t.Fatalf("url=%q, want=https://example.com/sub.txt", pe.AppError.URL)
	}
	if pe.AppError.Snippet == "" {
		t.Fatalf("snippet should not be empty")
	}
}
