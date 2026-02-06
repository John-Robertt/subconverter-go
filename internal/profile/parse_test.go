package profile

import (
	"errors"
	"testing"
)

func TestParseProfileYAML_OK(t *testing.T) {
	yml := `
version: 1
template:
  clash: "https://example.com/base_clash.yaml"
  shadowrocket: "https://example.com/base_sr.conf"
  surge: "https://example.com/base_surge.conf"
public_base_url: "https://sub.example.com/sub"
custom_proxy_group:
  - "PROXY` + "`" + `select` + "`" + `[]@all[]DIRECT"
  - "AUTO` + "`" + `url-test` + "`" + `(HK|SG)` + "`" + `https://www.gstatic.com/generate_204` + "`" + `300` + "`" + `50"
ruleset:
  - "DIRECT,https://example.com/LAN.list"
rule:
  - "MATCH,PROXY"
`

	p, err := ParseProfileYAML("https://example.com/profile.yaml", yml, "clash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Version != 1 {
		t.Fatalf("version=%d, want=1", p.Version)
	}
	if p.Template["clash"] == "" {
		t.Fatalf("template.clash should not be empty")
	}
	if p.PublicBaseURL != "https://sub.example.com/sub" {
		t.Fatalf("public_base_url=%q", p.PublicBaseURL)
	}
	if len(p.Groups) != 2 {
		t.Fatalf("groups=%d, want=2", len(p.Groups))
	}
	if p.Groups[0].Type != "select" || len(p.Groups[0].Members) == 0 {
		t.Fatalf("group0 parse failed: %+v", p.Groups[0])
	}
	if p.Groups[1].Type != "url-test" || p.Groups[1].Regex == nil {
		t.Fatalf("group1 parse failed: %+v", p.Groups[1])
	}
	if len(p.Rules) != 1 || p.Rules[0].Type != "MATCH" {
		t.Fatalf("rules parse failed: %+v", p.Rules)
	}
}

func TestParseProfileYAML_UnknownTopField_Strict(t *testing.T) {
	yml := `
version: 1
template:
  clash: "https://example.com/base.yaml"
unknown_field: 1
rule:
  - "MATCH,DIRECT"
`
	_, err := ParseProfileYAML("https://example.com/profile.yaml", yml, "clash")
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T: %v", err, err)
	}
	if pe.AppError.Code != "PROFILE_PARSE_ERROR" {
		t.Fatalf("code=%q, want=%q", pe.AppError.Code, "PROFILE_PARSE_ERROR")
	}
	if pe.AppError.Stage != "parse_profile" {
		t.Fatalf("stage=%q, want=%q", pe.AppError.Stage, "parse_profile")
	}
}

func TestParseProfileYAML_TemplateMissingTarget(t *testing.T) {
	yml := `
version: 1
template:
  shadowrocket: "https://example.com/base_sr.conf"
rule:
  - "MATCH,DIRECT"
`
	_, err := ParseProfileYAML("https://example.com/profile.yaml", yml, "clash")
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T: %v", err, err)
	}
	if pe.AppError.Code != "PROFILE_VALIDATE_ERROR" {
		t.Fatalf("code=%q, want=%q", pe.AppError.Code, "PROFILE_VALIDATE_ERROR")
	}
}

func TestParseProfileYAML_PublicBaseURLHasQuery(t *testing.T) {
	yml := `
version: 1
template:
  clash: "https://example.com/base.yaml"
public_base_url: "https://sub.example.com/sub?x=1"
rule:
  - "MATCH,DIRECT"
`
	_, err := ParseProfileYAML("https://example.com/profile.yaml", yml, "clash")
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T: %v", err, err)
	}
	if pe.AppError.Code != "PROFILE_VALIDATE_ERROR" {
		t.Fatalf("code=%q, want=%q", pe.AppError.Code, "PROFILE_VALIDATE_ERROR")
	}
}

func TestParseProfileYAML_UnsupportedGroupType(t *testing.T) {
	yml := `
version: 1
template:
  clash: "https://example.com/base.yaml"
custom_proxy_group:
  - "A` + "`" + `fallback` + "`" + `[]@all"
rule:
  - "MATCH,DIRECT"
`
	_, err := ParseProfileYAML("https://example.com/profile.yaml", yml, "clash")
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T: %v", err, err)
	}
	if pe.AppError.Code != "GROUP_UNSUPPORTED_TYPE" {
		t.Fatalf("code=%q, want=%q", pe.AppError.Code, "GROUP_UNSUPPORTED_TYPE")
	}
}

func TestParseProfileYAML_InvalidRulesetDirective(t *testing.T) {
	yml := `
version: 1
template:
  clash: "https://example.com/base.yaml"
ruleset:
  - "DIRECT https://example.com/a.list"
rule:
  - "MATCH,DIRECT"
`
	_, err := ParseProfileYAML("https://example.com/profile.yaml", yml, "clash")
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T: %v", err, err)
	}
	if pe.AppError.Code != "RULESET_PARSE_ERROR" {
		t.Fatalf("code=%q, want=%q", pe.AppError.Code, "RULESET_PARSE_ERROR")
	}
}

func TestParseProfileYAML_InlineRuleMissingAction(t *testing.T) {
	yml := `
version: 1
template:
  clash: "https://example.com/base.yaml"
rule:
  - "DOMAIN,example.com"
`
	_, err := ParseProfileYAML("https://example.com/profile.yaml", yml, "clash")
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T: %v", err, err)
	}
	if pe.AppError.Code != "RULE_PARSE_ERROR" {
		t.Fatalf("code=%q, want=%q", pe.AppError.Code, "RULE_PARSE_ERROR")
	}
}

func TestParseProfileYAML_SelectRegex_OK(t *testing.T) {
	yml := `
version: 1
template:
  clash: "https://example.com/base.yaml"
custom_proxy_group:
  - "HK` + "`" + `select` + "`" + `(HK|Hong Kong)"
rule:
  - "MATCH,DIRECT"
`
	p, err := ParseProfileYAML("https://example.com/profile.yaml", yml, "clash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Groups) != 1 {
		t.Fatalf("groups=%d, want=1", len(p.Groups))
	}
	g := p.Groups[0]
	if g.Type != "select" {
		t.Fatalf("group type=%q, want=%q", g.Type, "select")
	}
	if len(g.Members) != 0 {
		t.Fatalf("group members=%v, want empty", g.Members)
	}
	if g.Regex == nil || g.RegexRaw == "" {
		t.Fatalf("group regex missing: %+v", g)
	}
	if !g.Regex.MatchString("Hong Kong 01") {
		t.Fatalf("regex should match, got %q", g.RegexRaw)
	}
}

func TestParseProfileYAML_MissingMatchRule(t *testing.T) {
	yml := `
version: 1
template:
  clash: "https://example.com/base.yaml"
rule:
  - "DOMAIN,example.com,DIRECT"
`
	_, err := ParseProfileYAML("https://example.com/profile.yaml", yml, "clash")
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T: %v", err, err)
	}
	if pe.AppError.Code != "PROFILE_VALIDATE_ERROR" {
		t.Fatalf("code=%q, want=%q", pe.AppError.Code, "PROFILE_VALIDATE_ERROR")
	}
}
