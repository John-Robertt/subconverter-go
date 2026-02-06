package profile

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/John-Robertt/subconverter-go/internal/model"
	"github.com/John-Robertt/subconverter-go/internal/rules"
	"gopkg.in/yaml.v3"
)

type Spec struct {
	Version int

	Template      map[string]string
	PublicBaseURL string

	Groups  []GroupSpec
	Ruleset []RulesetSpec
	Rules   []model.Rule // inline rules
}

type GroupSpec struct {
	Raw  string
	Name string
	Type string // "select" | "url-test"

	// select
	Members []string

	// url-test
	RegexRaw    string
	Regex       *regexp.Regexp
	TestURL     string
	IntervalSec int

	ToleranceMS  int
	HasTolerance bool
}

type RulesetSpec struct {
	Raw    string
	Action string
	URL    string
}

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

type directiveError struct {
	Code    string
	Message string
	Hint    string
	Cause   error
}

func (e *directiveError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

func (e *directiveError) Unwrap() error { return e.Cause }

type rawProfile struct {
	Version          int               `yaml:"version"`
	Template         map[string]string `yaml:"template"`
	PublicBaseURL    string            `yaml:"public_base_url"`
	CustomProxyGroup []string          `yaml:"custom_proxy_group"`
	Ruleset          []string          `yaml:"ruleset"`
	Rule             []string          `yaml:"rule"`
}

// ParseProfileYAML parses and validates a profile YAML document.
//
// requiredTarget is optional. If non-empty, template must contain that key.
// stage is always "parse_profile" to match docs/spec/SPEC_HTTP_API.md.
func ParseProfileYAML(sourceURL string, content string, requiredTarget string) (*Spec, error) {
	var rp rawProfile
	if err := yamlDecodeStrict(content, &rp); err != nil {
		return nil, &ParseError{
			AppError: model.AppError{
				Code:    "PROFILE_PARSE_ERROR",
				Message: "profile YAML 解析失败",
				Stage:   "parse_profile",
				URL:     sourceURL,
				Snippet: truncateSnippet(content, 200),
			},
			Cause: err,
		}
	}

	if rp.Version != 1 {
		return nil, &ParseError{
			AppError: model.AppError{
				Code:    "PROFILE_VALIDATE_ERROR",
				Message: "profile version 必须为 1",
				Stage:   "parse_profile",
				URL:     sourceURL,
			},
		}
	}

	if len(rp.Template) == 0 {
		return nil, &ParseError{
			AppError: model.AppError{
				Code:    "PROFILE_VALIDATE_ERROR",
				Message: "template 不能为空",
				Stage:   "parse_profile",
				URL:     sourceURL,
				Hint:    "expected: template: {clash: ..., shadowrocket: ..., surge: ...}",
			},
		}
	}

	allowedTargets := map[string]struct{}{
		"clash":        {},
		"shadowrocket": {},
		"surge":        {},
		"quanx":        {},
	}
	for k, v := range rp.Template {
		if _, ok := allowedTargets[k]; !ok {
			return nil, &ParseError{
				AppError: model.AppError{
					Code:    "PROFILE_VALIDATE_ERROR",
					Message: fmt.Sprintf("template key 不支持：%s", k),
					Stage:   "parse_profile",
					URL:     sourceURL,
				},
			}
		}
		if err := validateHTTPURL(v); err != nil {
			return nil, &ParseError{
				AppError: model.AppError{
					Code:    "PROFILE_VALIDATE_ERROR",
					Message: fmt.Sprintf("template.%s URL 不合法", k),
					Stage:   "parse_profile",
					URL:     sourceURL,
					Snippet: v,
				},
				Cause: err,
			}
		}
	}
	if requiredTarget != "" {
		if _, ok := rp.Template[requiredTarget]; !ok {
			return nil, &ParseError{
				AppError: model.AppError{
					Code:    "PROFILE_VALIDATE_ERROR",
					Message: fmt.Sprintf("template 缺少 target=%s", requiredTarget),
					Stage:   "parse_profile",
					URL:     sourceURL,
				},
			}
		}
	}

	publicBaseURL := strings.TrimSpace(rp.PublicBaseURL)
	if publicBaseURL != "" {
		if err := validatePublicBaseURL(publicBaseURL); err != nil {
			return nil, &ParseError{
				AppError: model.AppError{
					Code:    "PROFILE_VALIDATE_ERROR",
					Message: "public_base_url 不合法",
					Stage:   "parse_profile",
					URL:     sourceURL,
					Snippet: publicBaseURL,
				},
				Cause: err,
			}
		}
	}

	groups := make([]GroupSpec, 0, len(rp.CustomProxyGroup))
	for _, raw := range rp.CustomProxyGroup {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		g, err := parseGroupDirective(raw)
		if err != nil {
			var de *directiveError
			if errors.As(err, &de) {
				return nil, &ParseError{
					AppError: model.AppError{
						Code:    de.Code,
						Message: de.Message,
						Stage:   "parse_profile",
						URL:     sourceURL,
						Snippet: raw,
						Hint:    de.Hint,
					},
					Cause: de.Cause,
				}
			}
			return nil, &ParseError{
				AppError: model.AppError{
					Code:    "GROUP_PARSE_ERROR",
					Message: "custom_proxy_group 解析失败",
					Stage:   "parse_profile",
					URL:     sourceURL,
					Snippet: raw,
				},
				Cause: err,
			}
		}
		groups = append(groups, g)
	}

	// Validate group names: non-empty, unique, not reserved.
	groupNames := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		if g.Name == "" {
			return nil, &ParseError{
				AppError: model.AppError{
					Code:    "GROUP_PARSE_ERROR",
					Message: "策略组名不能为空",
					Stage:   "parse_profile",
					URL:     sourceURL,
					Snippet: g.Raw,
				},
			}
		}
		if g.Name == "DIRECT" || g.Name == "REJECT" {
			return nil, &ParseError{
				AppError: model.AppError{
					Code:    "PROFILE_VALIDATE_ERROR",
					Message: "策略组名不能使用保留名 DIRECT/REJECT",
					Stage:   "parse_profile",
					URL:     sourceURL,
					Snippet: g.Raw,
				},
			}
		}
		if _, ok := groupNames[g.Name]; ok {
			return nil, &ParseError{
				AppError: model.AppError{
					Code:    "PROFILE_VALIDATE_ERROR",
					Message: fmt.Sprintf("重复的策略组名：%s", g.Name),
					Stage:   "parse_profile",
					URL:     sourceURL,
					Snippet: g.Raw,
				},
			}
		}
		groupNames[g.Name] = struct{}{}
	}

	// Validate select-group references.
	for _, g := range groups {
		if g.Type != "select" {
			continue
		}
		for _, m := range g.Members {
			if m == "@all" || m == "DIRECT" || m == "REJECT" {
				continue
			}
			if _, ok := groupNames[m]; !ok {
				return nil, &ParseError{
					AppError: model.AppError{
						Code:    "GROUP_PARSE_ERROR",
						Message: fmt.Sprintf("策略组引用不存在：%s", m),
						Stage:   "parse_profile",
						URL:     sourceURL,
						Snippet: g.Raw,
					},
				}
			}
		}
	}

	rulesets := make([]RulesetSpec, 0, len(rp.Ruleset))
	for _, raw := range rp.Ruleset {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		rs, err := parseRulesetDirective(raw)
		if err != nil {
			return nil, &ParseError{
				AppError: model.AppError{
					Code:    "RULESET_PARSE_ERROR",
					Message: "ruleset 指令解析失败",
					Stage:   "parse_profile",
					URL:     sourceURL,
					Snippet: raw,
				},
				Cause: err,
			}
		}
		rulesets = append(rulesets, rs)
	}

	inlineRules := make([]model.Rule, 0, len(rp.Rule))
	hasMatch := false
	for _, raw := range rp.Rule {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		r, err := rules.ParseInlineRule(raw)
		if err != nil {
			var re *rules.RuleError
			if errors.As(err, &re) {
				return nil, &ParseError{
					AppError: model.AppError{
						Code:    re.Code,
						Message: re.Message,
						Stage:   "parse_profile",
						URL:     sourceURL,
						Snippet: raw,
						Hint:    re.Hint,
					},
					Cause: re.Cause,
				}
			}
			return nil, &ParseError{
				AppError: model.AppError{
					Code:    "RULE_PARSE_ERROR",
					Message: "rule 指令解析失败",
					Stage:   "parse_profile",
					URL:     sourceURL,
					Snippet: raw,
				},
				Cause: err,
			}
		}
		if r.Type == "MATCH" {
			hasMatch = true
		}
		inlineRules = append(inlineRules, r)
	}
	if !hasMatch {
		return nil, &ParseError{
			AppError: model.AppError{
				Code:    "PROFILE_VALIDATE_ERROR",
				Message: "缺少兜底规则 MATCH,<ACTION>",
				Stage:   "parse_profile",
				URL:     sourceURL,
				Hint:    "add at end of rule: MATCH,PROXY",
			},
		}
	}

	return &Spec{
		Version:       rp.Version,
		Template:      rp.Template,
		PublicBaseURL: publicBaseURL,
		Groups:        groups,
		Ruleset:       rulesets,
		Rules:         inlineRules,
	}, nil
}

func yamlDecodeStrict(content string, out any) error {
	dec := yaml.NewDecoder(strings.NewReader(content))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return err
	}

	// Reject multi-document YAML to keep behavior deterministic.
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return errors.New("multiple YAML documents are not allowed")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func validateHTTPURL(s string) error {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return err
	}
	if u == nil || !u.IsAbs() {
		return errors.New("url must be absolute")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("scheme must be http/https")
	}
	return nil
}

func validatePublicBaseURL(s string) error {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return err
	}
	if u == nil || !u.IsAbs() {
		return errors.New("url must be absolute")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("scheme must be http/https")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return errors.New("public_base_url must not contain query/fragment")
	}
	return nil
}

func parseRulesetDirective(raw string) (RulesetSpec, error) {
	a, u, ok := strings.Cut(raw, ",")
	if !ok {
		return RulesetSpec{}, errors.New("expected: ACTION,URL")
	}
	action := strings.TrimSpace(a)
	urlStr := strings.TrimSpace(u)
	if action == "" || urlStr == "" {
		return RulesetSpec{}, errors.New("ACTION/URL must not be empty")
	}
	if err := validateHTTPURL(urlStr); err != nil {
		return RulesetSpec{}, err
	}
	return RulesetSpec{Raw: raw, Action: action, URL: urlStr}, nil
}

func parseGroupDirective(raw string) (GroupSpec, error) {
	parts := strings.Split(raw, "`")
	if len(parts) < 2 {
		return GroupSpec{}, &directiveError{
			Code:    "GROUP_PARSE_ERROR",
			Message: "custom_proxy_group 指令格式不合法",
			Hint:    "expected: <NAME>`select`[]... or <NAME>`url-test`<REGEX>`<URL>`<INTERVAL>[`<TOLERANCE>]",
		}
	}

	name := strings.TrimSpace(parts[0])
	typ := strings.TrimSpace(parts[1])
	if strings.ContainsAny(name, "\r\n\x00") {
		return GroupSpec{}, errors.New("group name contains control chars")
	}
	if name == "" || typ == "" {
		return GroupSpec{}, errors.New("group name/type must not be empty")
	}

	switch typ {
	case "select":
		if len(parts) != 3 {
			return GroupSpec{}, errors.New("select group must be: <NAME>`select`[]<MEMBER_1>[]<MEMBER_2>... or <NAME>`select`<REGEX>")
		}

		third := strings.TrimSpace(parts[2])
		if third == "" {
			return GroupSpec{}, errors.New("select group requires member list or regex")
		}

		// Two supported forms:
		// 1) explicit members: <NAME>`select`[]A[]B...
		// 2) regex filter:     <NAME>`select`(HK|SG|US)
		if !strings.HasPrefix(third, "[]") {
			re, err := regexp.Compile(third)
			if err != nil {
				return GroupSpec{}, &directiveError{
					Code:    "GROUP_PARSE_ERROR",
					Message: "select 正则不可编译",
					Hint:    "expected: <NAME>`select`(REGEX) or <NAME>`select`[]MEMBER...",
					Cause:   err,
				}
			}
			return GroupSpec{
				Raw:      raw,
				Name:     name,
				Type:     "select",
				RegexRaw: third,
				Regex:    re,
			}, nil
		}

		// Explicit members.
		toks := strings.Split(third, "[]")
		members := make([]string, 0, len(toks))
		for _, tok := range toks[1:] {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				return GroupSpec{}, errors.New("empty member in select group")
			}
			members = append(members, tok)
		}
		if len(members) == 0 {
			return GroupSpec{}, errors.New("select group requires at least 1 member")
		}
		return GroupSpec{
			Raw:     raw,
			Name:    name,
			Type:    "select",
			Members: members,
		}, nil
	case "url-test":
		// <NAME>`url-test`<REGEX>`<URL>`<INTERVAL_SEC>[`<TOLERANCE_MS>]
		if len(parts) != 5 && len(parts) != 6 {
			return GroupSpec{}, errors.New("url-test group must be: <NAME>`url-test`<REGEX>`<URL>`<INTERVAL_SEC>[`<TOLERANCE_MS>]")
		}
		regexRaw := parts[2]
		testURL := parts[3]
		intervalRaw := parts[4]
		if regexRaw == "" || testURL == "" || intervalRaw == "" {
			return GroupSpec{}, errors.New("url-test regex/url/interval must not be empty")
		}
		re, err := regexp.Compile(regexRaw)
		if err != nil {
			return GroupSpec{}, &directiveError{
				Code:    "GROUP_PARSE_ERROR",
				Message: "url-test 正则不可编译",
				Cause:   err,
			}
		}
		if err := validateHTTPURL(testURL); err != nil {
			return GroupSpec{}, err
		}
		intervalSec, err := strconv.Atoi(strings.TrimSpace(intervalRaw))
		if err != nil || intervalSec <= 0 {
			return GroupSpec{}, errors.New("url-test interval must be a positive integer")
		}
		var tol int
		var hasTol bool
		if len(parts) == 6 {
			hasTol = true
			tol, err = strconv.Atoi(strings.TrimSpace(parts[5]))
			if err != nil || tol < 0 {
				return GroupSpec{}, errors.New("url-test tolerance must be a non-negative integer")
			}
		}
		return GroupSpec{
			Raw:          raw,
			Name:         name,
			Type:         "url-test",
			RegexRaw:     regexRaw,
			Regex:        re,
			TestURL:      testURL,
			IntervalSec:  intervalSec,
			ToleranceMS:  tol,
			HasTolerance: hasTol,
		}, nil
	default:
		return GroupSpec{}, &directiveError{
			Code:    "GROUP_UNSUPPORTED_TYPE",
			Message: fmt.Sprintf("不支持的策略组类型：%s", typ),
		}
	}
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
