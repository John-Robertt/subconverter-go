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

func TestParseRulesetText_IPCIDR_NoResolveWithoutAction_OK(t *testing.T) {
	text := "IP-CIDR,1.1.1.1/32,no-resolve\n"
	rules, err := ParseRulesetText("https://example.com/r.list", text, "PROXY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("len=%d, want=1", len(rules))
	}
	if rules[0].Action != "PROXY" || !rules[0].NoResolve {
		t.Fatalf("got action=%q noResolve=%v, want action=%q noResolve=true", rules[0].Action, rules[0].NoResolve, "PROXY")
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

func TestParseInlineRule_IPCIDR_NoResolveWithoutAction_Error(t *testing.T) {
	_, err := ParseInlineRule("IP-CIDR,1.1.1.1/32,no-resolve")
	var re *RuleError
	if !errors.As(err, &re) {
		t.Fatalf("expected *RuleError, got %T: %v", err, err)
	}
	if re.Code != "RULE_PARSE_ERROR" {
		t.Fatalf("code=%q, want=%q", re.Code, "RULE_PARSE_ERROR")
	}
}

func TestParseInlineRule_UnsupportedType(t *testing.T) {
	_, err := ParseInlineRule("DST-PORT,443,DIRECT")
	var re *RuleError
	if !errors.As(err, &re) {
		t.Fatalf("expected *RuleError, got %T: %v", err, err)
	}
	if re.Code != "UNSUPPORTED_RULE_TYPE" {
		t.Fatalf("code=%q, want=%q", re.Code, "UNSUPPORTED_RULE_TYPE")
	}
}

func TestParseRulesetText_IPCIDR6_NoResolveWithoutAction_OK(t *testing.T) {
	text := "IP-CIDR6,2001:db8::/32,no-resolve\n"
	rules, err := ParseRulesetText("https://example.com/r.list", text, "PROXY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("len=%d, want=1", len(rules))
	}
	if rules[0].Action != "PROXY" || !rules[0].NoResolve {
		t.Fatalf("got action=%q noResolve=%v, want action=%q noResolve=true", rules[0].Action, rules[0].NoResolve, "PROXY")
	}
}
