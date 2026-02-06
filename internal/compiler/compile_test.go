package compiler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/John-Robertt/subconverter-go/internal/model"
	"github.com/John-Robertt/subconverter-go/internal/profile"
)

func TestCompile_DeterminismAndExpansion(t *testing.T) {
	rsText := "DOMAIN-SUFFIX,google.com\nIP-CIDR,1.1.1.1/32,DIRECT,no-resolve\n"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(rsText))
	}))
	defer ts.Close()

	subs := []model.Proxy{
		{Type: "ss", Name: "HK", Server: "Example.COM", Port: 8388, Cipher: "AES-128-GCM", Password: "pass"},
		{Type: "ss", Name: "HK", Server: "example.com", Port: 8388, Cipher: "aes-128-gcm", Password: "pass"}, // dup
		{Type: "ss", Name: "HK", Server: "example.com", Port: 8389, Cipher: "aes-128-gcm", Password: "pass"},
		{Type: "ss", Name: "a=b", Server: "b.com", Port: 1, Cipher: "aes-128-gcm", Password: "pass"},
		{Type: "ss", Name: "DIRECT", Server: "c.com", Port: 1, Cipher: "aes-128-gcm", Password: "pass"},
	}

	prof := &profile.Spec{
		Version: 1,
		Groups: []profile.GroupSpec{
			{Raw: "PROXY`select`[]@all[]DIRECT", Name: "PROXY", Type: "select", Members: []string{"@all", "DIRECT"}},
			{Raw: "AUTO`url-test`HK`https://www.gstatic.com/generate_204`300", Name: "AUTO", Type: "url-test", RegexRaw: "HK", Regex: regexp.MustCompile("HK"), TestURL: "https://www.gstatic.com/generate_204", IntervalSec: 300},
		},
		Ruleset: []profile.RulesetSpec{
			{Raw: "PROXY," + ts.URL, Action: "PROXY", URL: ts.URL},
		},
		Rules: []model.Rule{
			{Type: "DOMAIN", Value: "example.com", Action: "PROXY"},
			{Type: "MATCH", Action: "PROXY"},
		},
	}

	got, err := Compile(context.Background(), subs, prof, Options{ExpandRulesets: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Proxies) != 4 {
		t.Fatalf("proxies=%d, want=4", len(got.Proxies))
	}
	if got.Proxies[0].Name != "DIRECT-2" || got.Proxies[1].Name != "HK" || got.Proxies[2].Name != "HK-2" || got.Proxies[3].Name != "a-b" {
		t.Fatalf("proxy names=%q,%q,%q,%q", got.Proxies[0].Name, got.Proxies[1].Name, got.Proxies[2].Name, got.Proxies[3].Name)
	}

	if len(got.Groups) != 2 {
		t.Fatalf("groups=%d, want=2", len(got.Groups))
	}
	if got.Groups[0].Name != "PROXY" || got.Groups[0].Type != "select" {
		t.Fatalf("group0=%+v", got.Groups[0])
	}
	if want := 5; len(got.Groups[0].Members) != want {
		t.Fatalf("group0 members=%d, want=%d", len(got.Groups[0].Members), want)
	}
	if got.Groups[0].Members[0] != "DIRECT-2" || got.Groups[0].Members[1] != "HK" || got.Groups[0].Members[2] != "HK-2" || got.Groups[0].Members[3] != "a-b" || got.Groups[0].Members[4] != "DIRECT" {
		t.Fatalf("group0 members=%v", got.Groups[0].Members)
	}

	if got.Groups[1].Name != "AUTO" || got.Groups[1].Type != "url-test" {
		t.Fatalf("group1=%+v", got.Groups[1])
	}
	if len(got.Groups[1].Members) != 2 || got.Groups[1].Members[0] != "HK" || got.Groups[1].Members[1] != "HK-2" {
		t.Fatalf("group1 members=%v", got.Groups[1].Members)
	}

	if len(got.Rules) != 4 {
		t.Fatalf("rules=%d, want=4", len(got.Rules))
	}
	if got.Rules[0].Type != "DOMAIN-SUFFIX" || got.Rules[0].Action != "PROXY" {
		t.Fatalf("ruleset rule0=%+v", got.Rules[0])
	}
	if got.Rules[len(got.Rules)-1].Type != "MATCH" {
		t.Fatalf("last rule=%+v", got.Rules[len(got.Rules)-1])
	}
}

func TestCompile_URLTestEmpty(t *testing.T) {
	subs := []model.Proxy{
		{Type: "ss", Name: "HK", Server: "example.com", Port: 8388, Cipher: "aes-128-gcm", Password: "pass"},
	}
	prof := &profile.Spec{
		Version: 1,
		Groups: []profile.GroupSpec{
			{Raw: "AUTO`url-test`ZZZ`https://www.gstatic.com/generate_204`300", Name: "AUTO", Type: "url-test", RegexRaw: "ZZZ", Regex: regexp.MustCompile("ZZZ"), TestURL: "https://www.gstatic.com/generate_204", IntervalSec: 300},
		},
		Rules: []model.Rule{
			{Type: "MATCH", Action: "AUTO"},
		},
	}

	_, err := Compile(context.Background(), subs, prof, Options{ExpandRulesets: true})
	var ce *CompileError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *CompileError, got %T: %v", err, err)
	}
	if ce.AppError.Code != "GROUP_PARSE_ERROR" {
		t.Fatalf("code=%q, want=%q", ce.AppError.Code, "GROUP_PARSE_ERROR")
	}
}
