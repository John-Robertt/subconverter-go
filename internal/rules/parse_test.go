package rules

import (
	"errors"
	"testing"
)

func TestParseRulesetText_FillDefaultAction(t *testing.T) {
	text := "# comment\n\nDOMAIN-SUFFIX,google.com\nDOMAIN,example.com,DIRECT\n"
	rules, err := ParseRulesetText("https://example.com/r.list", text, "PROXY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("len=%d, want=2", len(rules))
	}
	if rules[0].Action != "PROXY" {
		t.Fatalf("action0=%q, want=%q", rules[0].Action, "PROXY")
	}
	if rules[1].Action != "DIRECT" {
		t.Fatalf("action1=%q, want=%q", rules[1].Action, "DIRECT")
	}
}

func TestParseRulesetText_IPCIDR_AmbiguousNoResolveWithoutAction(t *testing.T) {
	text := "IP-CIDR,1.1.1.1/32,no-resolve\n"
	_, err := ParseRulesetText("https://example.com/r.list", text, "PROXY")
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T: %v", err, err)
	}
	if pe.AppError.Code != "RULE_PARSE_ERROR" {
		t.Fatalf("code=%q, want=%q", pe.AppError.Code, "RULE_PARSE_ERROR")
	}
	if pe.AppError.Stage != "parse_ruleset" {
		t.Fatalf("stage=%q, want=%q", pe.AppError.Stage, "parse_ruleset")
	}
	if pe.AppError.Line != 1 {
		t.Fatalf("line=%d, want=1", pe.AppError.Line)
	}
	if pe.AppError.URL == "" || pe.AppError.Snippet == "" {
		t.Fatalf("expected url/snippet to be set")
	}
}

func TestParseRulesetText_DisallowMatch(t *testing.T) {
	text := "MATCH,PROXY\n"
	_, err := ParseRulesetText("https://example.com/r.list", text, "PROXY")
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T: %v", err, err)
	}
	if pe.AppError.Code != "RULESET_PARSE_ERROR" {
		t.Fatalf("code=%q, want=%q", pe.AppError.Code, "RULESET_PARSE_ERROR")
	}
}

func TestParseInlineRule_RequireAction(t *testing.T) {
	_, err := ParseInlineRule("DOMAIN,example.com")
	var re *RuleError
	if !errors.As(err, &re) {
		t.Fatalf("expected *RuleError, got %T: %v", err, err)
	}
	if re.Code != "RULE_PARSE_ERROR" {
		t.Fatalf("code=%q, want=%q", re.Code, "RULE_PARSE_ERROR")
	}
}

func TestParseInlineRule_UnsupportedType(t *testing.T) {
	_, err := ParseInlineRule("PROCESS-NAME,foo,DIRECT")
	var re *RuleError
	if !errors.As(err, &re) {
		t.Fatalf("expected *RuleError, got %T: %v", err, err)
	}
	if re.Code != "UNSUPPORTED_RULE_TYPE" {
		t.Fatalf("code=%q, want=%q", re.Code, "UNSUPPORTED_RULE_TYPE")
	}
}
