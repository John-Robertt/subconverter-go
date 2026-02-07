package rules

import (
	"errors"
	"testing"
)

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
