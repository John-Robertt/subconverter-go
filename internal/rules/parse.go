package rules

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/John-Robertt/subconverter-go/internal/model"
)

type RuleError struct {
	Code    string
	Message string
	Hint    string
	Cause   error
}

func (e *RuleError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

func (e *RuleError) Unwrap() error { return e.Cause }

// ParseInlineRule parses a single inline rule line from profile.rule.
//
// v1: all rule types require explicit ACTION, except MATCH which is "MATCH,<ACTION>".
func ParseInlineRule(line string) (model.Rule, error) {
	line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if line == "" {
		return model.Rule{}, &RuleError{Code: "RULE_PARSE_ERROR", Message: "rule line is empty"}
	}
	if strings.HasPrefix(line, "#") {
		return model.Rule{}, &RuleError{Code: "RULE_PARSE_ERROR", Message: "rule line is comment"}
	}
	return parseRuleLine(line)
}

func parseRuleLine(line string) (model.Rule, error) {
	parts := strings.Split(line, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) == 0 || parts[0] == "" {
		return model.Rule{}, &RuleError{Code: "RULE_PARSE_ERROR", Message: "规则类型不能为空"}
	}

	typ := strings.ToUpper(parts[0])
	switch typ {
	case "DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "GEOIP", "PROCESS-NAME", "URL-REGEX":
		return parseSimple3(typ, parts)
	case "IP-CIDR":
		return parseIPCidr(parts)
	case "IP-CIDR6":
		return parseIPCidr6(parts)
	case "MATCH":
		if len(parts) != 2 || parts[1] == "" {
			return model.Rule{}, &RuleError{
				Code:    "RULE_PARSE_ERROR",
				Message: "MATCH 规则必须是 MATCH,<ACTION>",
			}
		}
		return model.Rule{Type: "MATCH", Action: parts[1]}, nil
	default:
		return model.Rule{}, &RuleError{
			Code:    "UNSUPPORTED_RULE_TYPE",
			Message: fmt.Sprintf("不支持的规则类型：%s", typ),
		}
	}
}

func parseSimple3(typ string, parts []string) (model.Rule, error) {
	if len(parts) == 2 {
		return model.Rule{}, &RuleError{
			Code:    "RULE_PARSE_ERROR",
			Message: "规则缺少 ACTION",
			Hint:    "expected: TYPE,VALUE,ACTION",
		}
	}
	if len(parts) != 3 {
		return model.Rule{}, &RuleError{
			Code:    "RULE_PARSE_ERROR",
			Message: "规则字段数量不合法",
			Hint:    "expected: TYPE,VALUE,ACTION",
		}
	}
	if parts[1] == "" || parts[2] == "" {
		return model.Rule{}, &RuleError{Code: "RULE_PARSE_ERROR", Message: "规则 VALUE/ACTION 不能为空"}
	}
	return model.Rule{Type: typ, Value: parts[1], Action: parts[2]}, nil
}

func parseIPCidr(parts []string) (model.Rule, error) {
	switch len(parts) {
	case 3:
		if parts[2] == "" {
			return model.Rule{}, &RuleError{Code: "RULE_PARSE_ERROR", Message: "IP-CIDR 的 ACTION 不能为空"}
		}
		if strings.EqualFold(parts[2], "no-resolve") {
			// Inline rule requires explicit ACTION, so "no-resolve" here is ambiguous.
			return model.Rule{}, &RuleError{
				Code:    "RULE_PARSE_ERROR",
				Message: "IP-CIDR 缺少 ACTION（不允许仅写 no-resolve）",
				Hint:    "expected: IP-CIDR,CIDR,ACTION[,no-resolve]",
			}
		}
		if err := validateIPv4CIDR(parts[1]); err != nil {
			return model.Rule{}, &RuleError{
				Code:    "RULE_PARSE_ERROR",
				Message: "IP-CIDR 的 CIDR 不合法",
				Hint:    "expected: IPv4 CIDR, e.g. 1.2.3.4/32",
				Cause:   err,
			}
		}
		return model.Rule{Type: "IP-CIDR", Value: parts[1], Action: parts[2]}, nil
	case 4:
		if parts[2] == "" {
			return model.Rule{}, &RuleError{Code: "RULE_PARSE_ERROR", Message: "IP-CIDR 的 ACTION 不能为空"}
		}
		if !strings.EqualFold(parts[3], "no-resolve") {
			return model.Rule{}, &RuleError{
				Code:    "RULE_PARSE_ERROR",
				Message: "IP-CIDR 的可选项仅支持 no-resolve",
				Hint:    "expected: IP-CIDR,CIDR,ACTION[,no-resolve]",
			}
		}
		if err := validateIPv4CIDR(parts[1]); err != nil {
			return model.Rule{}, &RuleError{
				Code:    "RULE_PARSE_ERROR",
				Message: "IP-CIDR 的 CIDR 不合法",
				Hint:    "expected: IPv4 CIDR, e.g. 1.2.3.4/32",
				Cause:   err,
			}
		}
		return model.Rule{Type: "IP-CIDR", Value: parts[1], Action: parts[2], NoResolve: true}, nil
	default:
		return model.Rule{}, &RuleError{
			Code:    "RULE_PARSE_ERROR",
			Message: "IP-CIDR 规则字段数量不合法",
			Hint:    "expected: IP-CIDR,CIDR,ACTION[,no-resolve]",
		}
	}
}

func parseIPCidr6(parts []string) (model.Rule, error) {
	switch len(parts) {
	case 3:
		if parts[2] == "" {
			return model.Rule{}, &RuleError{Code: "RULE_PARSE_ERROR", Message: "IP-CIDR6 的 ACTION 不能为空"}
		}
		if strings.EqualFold(parts[2], "no-resolve") {
			return model.Rule{}, &RuleError{
				Code:    "RULE_PARSE_ERROR",
				Message: "IP-CIDR6 缺少 ACTION（不允许仅写 no-resolve）",
				Hint:    "expected: IP-CIDR6,CIDR,ACTION[,no-resolve]",
			}
		}
		if err := validateIPv6CIDR(parts[1]); err != nil {
			return model.Rule{}, &RuleError{
				Code:    "RULE_PARSE_ERROR",
				Message: "IP-CIDR6 的 CIDR 不合法",
				Hint:    "expected: IPv6 CIDR, e.g. 2001:db8::/32",
				Cause:   err,
			}
		}
		return model.Rule{Type: "IP-CIDR6", Value: parts[1], Action: parts[2]}, nil
	case 4:
		if parts[2] == "" {
			return model.Rule{}, &RuleError{Code: "RULE_PARSE_ERROR", Message: "IP-CIDR6 的 ACTION 不能为空"}
		}
		if !strings.EqualFold(parts[3], "no-resolve") {
			return model.Rule{}, &RuleError{
				Code:    "RULE_PARSE_ERROR",
				Message: "IP-CIDR6 的可选项仅支持 no-resolve",
				Hint:    "expected: IP-CIDR6,CIDR,ACTION[,no-resolve]",
			}
		}
		if err := validateIPv6CIDR(parts[1]); err != nil {
			return model.Rule{}, &RuleError{
				Code:    "RULE_PARSE_ERROR",
				Message: "IP-CIDR6 的 CIDR 不合法",
				Hint:    "expected: IPv6 CIDR, e.g. 2001:db8::/32",
				Cause:   err,
			}
		}
		return model.Rule{Type: "IP-CIDR6", Value: parts[1], Action: parts[2], NoResolve: true}, nil
	default:
		return model.Rule{}, &RuleError{
			Code:    "RULE_PARSE_ERROR",
			Message: "IP-CIDR6 规则字段数量不合法",
			Hint:    "expected: IP-CIDR6,CIDR,ACTION[,no-resolve]",
		}
	}
}

func validateIPv4CIDR(s string) error {
	ip, _, err := net.ParseCIDR(strings.TrimSpace(s))
	if err != nil {
		return err
	}
	if ip == nil || ip.To4() == nil {
		return errors.New("not an ipv4 cidr")
	}
	return nil
}

func validateIPv6CIDR(s string) error {
	ip, _, err := net.ParseCIDR(strings.TrimSpace(s))
	if err != nil {
		return err
	}
	if ip == nil || ip.To16() == nil || ip.To4() != nil {
		return errors.New("not an ipv6 cidr")
	}
	return nil
}
