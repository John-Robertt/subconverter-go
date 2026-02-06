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

type ParseError struct {
	AppError model.AppError
	Cause    error
}

func (e *ParseError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.AppError.Code, e.AppError.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.AppError.Code, e.AppError.Message, e.Cause)
}

func (e *ParseError) Unwrap() error { return e.Cause }

// ParseRulesetText parses a ruleset file (Clash classical lines).
// It allows missing ACTION in each line and fills it using defaultAction.
//
// stage is always "parse_ruleset" to match docs/spec/SPEC_HTTP_API.md.
func ParseRulesetText(sourceURL string, text string, defaultAction string) ([]model.Rule, error) {
	if strings.TrimSpace(defaultAction) == "" {
		return nil, &ParseError{
			AppError: model.AppError{
				Code:    "RULESET_PARSE_ERROR",
				Message: "ruleset default action 不能为空",
				Stage:   "parse_ruleset",
				URL:     sourceURL,
			},
		}
	}

	lines := strings.Split(text, "\n")
	out := make([]model.Rule, 0, len(lines))
	for i, raw := range lines {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}

		r, err := parseRuleLine(line, ruleParseOptions{
			AllowNoAction: true,
			DefaultAction: defaultAction,
			AllowMatch:    false,
		})
		if err != nil {
			var rerr *RuleError
			if errors.As(err, &rerr) {
				return nil, &ParseError{
					AppError: model.AppError{
						Code:    rerr.Code,
						Message: rerr.Message,
						Stage:   "parse_ruleset",
						URL:     sourceURL,
						Line:    i + 1,
						Snippet: truncateSnippet(raw, 200),
						Hint:    rerr.Hint,
					},
					Cause: rerr.Cause,
				}
			}

			return nil, &ParseError{
				AppError: model.AppError{
					Code:    "RULE_PARSE_ERROR",
					Message: "invalid rule line",
					Stage:   "parse_ruleset",
					URL:     sourceURL,
					Line:    i + 1,
					Snippet: truncateSnippet(raw, 200),
				},
				Cause: err,
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// ParseInlineRule parses a single inline rule line. ACTION is required.
// Caller is expected to attach proper stage/url/line if needed.
func ParseInlineRule(line string) (model.Rule, error) {
	line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if line == "" {
		return model.Rule{}, &RuleError{Code: "RULE_PARSE_ERROR", Message: "rule line is empty"}
	}
	if strings.HasPrefix(line, "#") {
		return model.Rule{}, &RuleError{Code: "RULE_PARSE_ERROR", Message: "rule line is comment"}
	}
	return parseRuleLine(line, ruleParseOptions{
		AllowNoAction: false,
		DefaultAction: "",
		AllowMatch:    true,
	})
}

type ruleParseOptions struct {
	AllowNoAction bool
	DefaultAction string
	AllowMatch    bool
}

func parseRuleLine(line string, opt ruleParseOptions) (model.Rule, error) {
	parts := strings.Split(line, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) == 0 || parts[0] == "" {
		return model.Rule{}, &RuleError{Code: "RULE_PARSE_ERROR", Message: "规则类型不能为空"}
	}

	typ := strings.ToUpper(parts[0])
	switch typ {
	case "DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "GEOIP":
		return parseSimple3(typ, parts, opt)
	case "IP-CIDR":
		return parseIPCidr(parts, opt)
	case "MATCH":
		if !opt.AllowMatch {
			return model.Rule{}, &RuleError{
				Code:    "RULESET_PARSE_ERROR",
				Message: "ruleset 不允许包含 MATCH 规则",
				Hint:    "move MATCH into profile.rule (inline rule)",
			}
		}
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

func parseSimple3(typ string, parts []string, opt ruleParseOptions) (model.Rule, error) {
	switch len(parts) {
	case 2:
		if !opt.AllowNoAction {
			return model.Rule{}, &RuleError{
				Code:    "RULE_PARSE_ERROR",
				Message: "规则缺少 ACTION",
				Hint:    "expected: TYPE,VALUE,ACTION",
			}
		}
		if parts[1] == "" {
			return model.Rule{}, &RuleError{Code: "RULE_PARSE_ERROR", Message: "规则 VALUE 不能为空"}
		}
		return model.Rule{Type: typ, Value: parts[1], Action: opt.DefaultAction}, nil
	case 3:
		if parts[1] == "" || parts[2] == "" {
			return model.Rule{}, &RuleError{Code: "RULE_PARSE_ERROR", Message: "规则 VALUE/ACTION 不能为空"}
		}
		return model.Rule{Type: typ, Value: parts[1], Action: parts[2]}, nil
	default:
		return model.Rule{}, &RuleError{
			Code:    "RULE_PARSE_ERROR",
			Message: "规则字段数量不合法",
			Hint:    "expected: TYPE,VALUE[,ACTION]",
		}
	}
}

func parseIPCidr(parts []string, opt ruleParseOptions) (model.Rule, error) {
	switch len(parts) {
	case 2:
		if !opt.AllowNoAction {
			return model.Rule{}, &RuleError{
				Code:    "RULE_PARSE_ERROR",
				Message: "IP-CIDR 规则缺少 ACTION",
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
		return model.Rule{Type: "IP-CIDR", Value: parts[1], Action: opt.DefaultAction}, nil
	case 3:
		if parts[2] == "" {
			return model.Rule{}, &RuleError{Code: "RULE_PARSE_ERROR", Message: "IP-CIDR 的 ACTION 不能为空"}
		}
		if strings.EqualFold(parts[2], "no-resolve") {
			// Ambiguous: missing action but has option.
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

func truncateSnippet(s string, max int) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max]
}
