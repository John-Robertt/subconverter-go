package compiler

import (
	"errors"
	"reflect"
	"regexp"
	"testing"

	"github.com/John-Robertt/subconverter-go/internal/model"
	"github.com/John-Robertt/subconverter-go/internal/profile"
)

func TestCompile_DeterminismAndRulesetRefs(t *testing.T) {
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
			{Raw: "PROXY,https://example.com/r.list", Action: "PROXY", URL: "https://example.com/r.list"},
		},
		Rules: []model.Rule{
			{Type: "DOMAIN", Value: "example.com", Action: "PROXY"},
			{Type: "MATCH", Action: "PROXY"},
		},
	}

	got, err := Compile(subs, prof)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Proxies) != 4 {
		t.Fatalf("proxies=%d, want=4", len(got.Proxies))
	}
	seenIDs := make(map[string]struct{}, len(got.Proxies))
	for _, p := range got.Proxies {
		if p.ID == "" {
			t.Fatalf("proxy id should not be empty: %+v", p)
		}
		if _, ok := seenIDs[p.ID]; ok {
			t.Fatalf("duplicate proxy id: %s", p.ID)
		}
		seenIDs[p.ID] = struct{}{}
	}
	// Proxies preserve subscription merge order (no sorting).
	if got.Proxies[0].Name != "HK" || got.Proxies[1].Name != "HK-2" || got.Proxies[2].Name != "a-b" || got.Proxies[3].Name != "DIRECT-2" {
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
	if !reflect.DeepEqual(got.Groups[0].Members, []model.MemberRef{
		proxyRef(got.Proxies[0].ID),
		proxyRef(got.Proxies[1].ID),
		proxyRef(got.Proxies[2].ID),
		proxyRef(got.Proxies[3].ID),
		builtinRef("DIRECT"),
	}) {
		t.Fatalf("group0 members=%v", got.Groups[0].Members)
	}

	if got.Groups[1].Name != "AUTO" || got.Groups[1].Type != "url-test" {
		t.Fatalf("group1=%+v", got.Groups[1])
	}
	if !reflect.DeepEqual(got.Groups[1].Members, []model.MemberRef{
		proxyRef(got.Proxies[0].ID),
		proxyRef(got.Proxies[1].ID),
	}) {
		t.Fatalf("group1 members=%v", got.Groups[1].Members)
	}

	if len(got.RulesetRefs) != 1 {
		t.Fatalf("rulesetRefs=%d, want=1", len(got.RulesetRefs))
	}
	if got.RulesetRefs[0].Action != "PROXY" || got.RulesetRefs[0].URL != "https://example.com/r.list" {
		t.Fatalf("rulesetRef0=%+v", got.RulesetRefs[0])
	}

	if len(got.Rules) != 2 {
		t.Fatalf("rules=%d, want=2", len(got.Rules))
	}
	if got.Rules[0].Type != "DOMAIN" || got.Rules[0].Value != "example.com" || got.Rules[0].Action != "PROXY" {
		t.Fatalf("rule0=%+v", got.Rules[0])
	}
	if got.Rules[len(got.Rules)-1].Type != "MATCH" {
		t.Fatalf("last rule=%+v", got.Rules[len(got.Rules)-1])
	}
}

func TestCompile_PreserveSubscriptionOrder_NoSort(t *testing.T) {
	subs := []model.Proxy{
		{Type: "ss", Name: "B", Server: "b.example.com", Port: 1, Cipher: "aes-128-gcm", Password: "pass"},
		{Type: "ss", Name: "A", Server: "a.example.com", Port: 2, Cipher: "aes-128-gcm", Password: "pass"},
	}

	prof := &profile.Spec{
		Version: 1,
		Groups: []profile.GroupSpec{
			{Raw: "G`select`[]@all", Name: "G", Type: "select", Members: []string{"@all"}},
			{Raw: "R`select`(A|B)", Name: "R", Type: "select", RegexRaw: "(A|B)", Regex: regexp.MustCompile("(A|B)")},
			{Raw: "U`url-test`(A|B)`http://example.com/204`300", Name: "U", Type: "url-test", RegexRaw: "(A|B)", Regex: regexp.MustCompile("(A|B)"), TestURL: "http://example.com/204", IntervalSec: 300},
		},
		Rules: []model.Rule{
			{Type: "MATCH", Action: "G"},
		},
	}

	got, err := Compile(subs, prof)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotProxyNames := make([]string, 0, len(got.Proxies))
	for _, p := range got.Proxies {
		gotProxyNames = append(gotProxyNames, p.Name)
	}
	if len(got.Proxies) != 2 || got.Proxies[0].Name != "B" || got.Proxies[1].Name != "A" {
		t.Fatalf("proxies=%v, want=[B A]", gotProxyNames)
	}

	if len(got.Groups) != 3 {
		t.Fatalf("groups=%d, want=3", len(got.Groups))
	}

	if got.Groups[0].Name != "G" || got.Groups[0].Type != "select" {
		t.Fatalf("group0=%+v", got.Groups[0])
	}
	if !reflect.DeepEqual(got.Groups[0].Members, []model.MemberRef{
		proxyRef(got.Proxies[0].ID),
		proxyRef(got.Proxies[1].ID),
	}) {
		t.Fatalf("G members=%v, want=[B A]", got.Groups[0].Members)
	}

	if got.Groups[1].Name != "R" || got.Groups[1].Type != "select" {
		t.Fatalf("group1=%+v", got.Groups[1])
	}
	if !reflect.DeepEqual(got.Groups[1].Members, []model.MemberRef{
		proxyRef(got.Proxies[0].ID),
		proxyRef(got.Proxies[1].ID),
	}) {
		t.Fatalf("R members=%v, want=[B A]", got.Groups[1].Members)
	}

	if got.Groups[2].Name != "U" || got.Groups[2].Type != "url-test" {
		t.Fatalf("group2=%+v", got.Groups[2])
	}
	if !reflect.DeepEqual(got.Groups[2].Members, []model.MemberRef{
		proxyRef(got.Proxies[0].ID),
		proxyRef(got.Proxies[1].ID),
	}) {
		t.Fatalf("U members=%v, want=[B A]", got.Groups[2].Members)
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

	_, err := Compile(subs, prof)
	var ce *CompileError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *CompileError, got %T: %v", err, err)
	}
	if ce.AppError.Code != "GROUP_PARSE_ERROR" {
		t.Fatalf("code=%q, want=%q", ce.AppError.Code, "GROUP_PARSE_ERROR")
	}
}

func TestCompile_SelectRegexMembers(t *testing.T) {
	subs := []model.Proxy{
		{Type: "ss", Name: "HK-A", Server: "hk.example.com", Port: 1, Cipher: "aes-128-gcm", Password: "pass"},
		{Type: "ss", Name: "HK-B", Server: "hk2.example.com", Port: 2, Cipher: "aes-128-gcm", Password: "pass"},
		{Type: "ss", Name: "SG", Server: "sg.example.com", Port: 3, Cipher: "aes-128-gcm", Password: "pass"},
	}
	prof := &profile.Spec{
		Version: 1,
		Groups: []profile.GroupSpec{
			{Raw: "HK`select`HK", Name: "HK", Type: "select", RegexRaw: "HK", Regex: regexp.MustCompile("HK")},
		},
		Rules: []model.Rule{
			{Type: "MATCH", Action: "DIRECT"},
		},
	}

	got, err := Compile(subs, prof)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Groups) != 1 {
		t.Fatalf("groups=%d, want=1", len(got.Groups))
	}
	if got.Groups[0].Name != "HK" || got.Groups[0].Type != "select" {
		t.Fatalf("group=%+v", got.Groups[0])
	}
	if !reflect.DeepEqual(got.Groups[0].Members, []model.MemberRef{
		proxyRef(got.Proxies[0].ID),
		proxyRef(got.Proxies[1].ID),
	}) {
		t.Fatalf("members=%v, want=[HK-A HK-B]", got.Groups[0].Members)
	}
}

func proxyRef(id string) model.MemberRef {
	return model.MemberRef{Kind: model.MemberRefProxy, Value: id}
}

func builtinRef(name string) model.MemberRef {
	return model.MemberRef{Kind: model.MemberRefBuiltin, Value: name}
}
