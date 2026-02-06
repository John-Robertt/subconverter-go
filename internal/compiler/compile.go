package compiler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/John-Robertt/subconverter-go/internal/fetch"
	"github.com/John-Robertt/subconverter-go/internal/model"
	"github.com/John-Robertt/subconverter-go/internal/profile"
	"github.com/John-Robertt/subconverter-go/internal/rules"
)

type Result struct {
	Proxies []model.Proxy
	Groups  []model.Group
	Rules   []model.Rule
	RulesetRefs []RulesetRef
}

type Options struct {
	// ExpandRulesets controls whether profile.ruleset is fetched and expanded into
	// inline rule lines.
	//
	// - true: fetch+parse ruleset files and append them before profile.rule
	// - false: keep ruleset as "remote reference" (renderer decides how to output)
	ExpandRulesets bool
}

type CompileError struct {
	AppError model.AppError
	Cause    error
}

func (e *CompileError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.AppError.Code, e.AppError.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.AppError.Code, e.AppError.Message, e.Cause)
}

func (e *CompileError) Unwrap() error { return e.Cause }

// NormalizeSubscriptionProxies applies v1 determinism rules to subscription proxies:
// normalization + dedup + deterministic naming + ordering.
//
// It is used by both mode=config (compile) and mode=list output.
func NormalizeSubscriptionProxies(subs []model.Proxy) ([]model.Proxy, error) {
	return compileProxies(subs)
}

func Compile(ctx context.Context, subs []model.Proxy, prof *profile.Spec, opt Options) (*Result, error) {
	if prof == nil {
		return nil, &CompileError{
			AppError: model.AppError{
				Code:    "PROFILE_VALIDATE_ERROR",
				Message: "profile 不能为空",
				Stage:   "compile",
			},
		}
	}

	groupNameSet := make(map[string]struct{}, len(prof.Groups))
	for _, g := range prof.Groups {
		groupNameSet[g.Name] = struct{}{}
	}

	proxies, err := compileProxies(subs)
	if err != nil {
		return nil, err
	}
	if len(proxies) == 0 {
		return nil, &CompileError{
			AppError: model.AppError{
				Code:    "SUB_PARSE_ERROR",
				Message: "没有任何可用节点",
				Stage:   "compile",
			},
		}
	}

	// Group name namespace must not conflict with proxy names (SPEC_DETERMINISM.md).
	proxyNameSet := make(map[string]struct{}, len(proxies))
	for _, p := range proxies {
		proxyNameSet[p.Name] = struct{}{}
	}
	for name := range groupNameSet {
		if _, ok := proxyNameSet[name]; ok {
			return nil, &CompileError{
				AppError: model.AppError{
					Code:    "PROFILE_VALIDATE_ERROR",
					Message: fmt.Sprintf("策略组名与节点名冲突：%s", name),
					Stage:   "compile",
				},
			}
		}
	}

	groups, err := compileGroups(proxies, prof.Groups)
	if err != nil {
		return nil, err
	}

	rulesOut, rulesetRefs, err := compileRules(ctx, groupNameSet, prof, opt.ExpandRulesets)
	if err != nil {
		return nil, err
	}

	return &Result{
		Proxies: proxies,
		Groups:  groups,
		Rules:   rulesOut,
		RulesetRefs: rulesetRefs,
	}, nil
}

type RulesetRef struct {
	Raw    string
	Action string
	URL    string
}

func compileProxies(in []model.Proxy) ([]model.Proxy, error) {
	// 1) Normalize
	normalized := make([]model.Proxy, 0, len(in))
	for _, p := range in {
		p2, err := normalizeProxy(p)
		if err != nil {
			return nil, &CompileError{
				AppError: model.AppError{
					Code:    "SUB_PARSE_ERROR",
					Message: "节点字段不合法",
					Stage:   "compile",
					Snippet: p.Name,
				},
				Cause: err,
			}
		}
		normalized = append(normalized, p2)
	}

	// 2) Dedup (keep first occurrence in merge order).
	seen := make(map[string]struct{}, len(normalized))
	deduped := make([]model.Proxy, 0, len(normalized))
	for _, p := range normalized {
		key := dedupKey(p)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, p)
	}

	// 3) Deterministic naming (merge-order based).
	used := make(map[string]struct{}, len(deduped))
	for i := range deduped {
		base := strings.TrimSpace(deduped[i].Name)
		if base == "" {
			base = fmt.Sprintf("%s:%d", deduped[i].Server, deduped[i].Port)
		}
		base = strings.ReplaceAll(base, "=", "-")

		name := base
		if name == "DIRECT" || name == "REJECT" {
			name = ""
		}

		if name != "" {
			if _, ok := used[name]; ok {
				name = ""
			}
		}

		if name == "" {
			// Pick baseName-N starting from 2.
			for n := 2; ; n++ {
				try := fmt.Sprintf("%s-%d", base, n)
				if _, ok := used[try]; ok {
					continue
				}
				name = try
				break
			}
		}

		deduped[i].Name = name
		used[name] = struct{}{}
	}

	// 4) Sort by final Name (stable output).
	sort.Slice(deduped, func(i, j int) bool {
		return deduped[i].Name < deduped[j].Name
	})
	return deduped, nil
}

func normalizeProxy(p model.Proxy) (model.Proxy, error) {
	if p.Type != "ss" {
		return model.Proxy{}, errors.New("only ss proxies are supported in v1")
	}

	p.Name = strings.TrimSpace(p.Name)
	if strings.ContainsAny(p.Name, "\r\n\x00") {
		return model.Proxy{}, errors.New("proxy name contains control chars")
	}

	p.Server = strings.TrimSpace(p.Server)
	if p.Server == "" {
		return model.Proxy{}, errors.New("empty server")
	}
	p.Server = strings.ToLower(p.Server)

	p.Cipher = strings.ToLower(strings.TrimSpace(p.Cipher))
	if p.Cipher == "" {
		return model.Proxy{}, errors.New("empty cipher")
	}
	p.Password = strings.TrimSpace(p.Password)
	if p.Password == "" {
		return model.Proxy{}, errors.New("empty password")
	}

	p.PluginName = strings.TrimSpace(p.PluginName)
	if len(p.PluginOpts) > 0 {
		opts := make([]model.KV, 0, len(p.PluginOpts))
		for _, kv := range p.PluginOpts {
			k := strings.TrimSpace(kv.Key)
			v := strings.TrimSpace(kv.Value)
			opts = append(opts, model.KV{Key: k, Value: v})
		}
		p.PluginOpts = opts
	}

	return p, nil
}

func dedupKey(p model.Proxy) string {
	var b strings.Builder
	// v1: only ss
	b.WriteString("ss\n")
	b.WriteString(p.Server)
	b.WriteByte('\n')
	b.WriteString(fmt.Sprintf("%d", p.Port))
	b.WriteByte('\n')
	b.WriteString(p.Cipher)
	b.WriteByte('\n')
	b.WriteString(p.Password)
	b.WriteByte('\n')
	b.WriteString(p.PluginName)
	b.WriteByte('\n')
	for _, kv := range p.PluginOpts {
		b.WriteString(kv.Key)
		b.WriteByte('=')
		b.WriteString(kv.Value)
		b.WriteByte(';')
	}
	return b.String()
}

func compileGroups(proxies []model.Proxy, groupSpecs []profile.GroupSpec) ([]model.Group, error) {
	allNames := make([]string, 0, len(proxies))
	for _, p := range proxies {
		allNames = append(allNames, p.Name)
	}

	out := make([]model.Group, 0, len(groupSpecs))
	for _, gs := range groupSpecs {
		switch gs.Type {
		case "select":
			var members []string
			if len(gs.Members) > 0 {
				// Explicit member list form: []A[]B[]@all...
				members = make([]string, 0, len(gs.Members)+len(allNames))
				for _, m := range gs.Members {
					if m == "@all" {
						members = append(members, allNames...)
					} else {
						members = append(members, m)
					}
				}
			} else if gs.Regex != nil {
				// Regex filter form: <NAME>`select`(REGEX)
				// Members are matched proxies only, in deterministic name order.
				members = make([]string, 0)
				for _, name := range allNames {
					if gs.Regex.MatchString(name) {
						members = append(members, name)
					}
				}
			} else {
				members = nil
			}
			if len(members) == 0 {
				return nil, &CompileError{
					AppError: model.AppError{
						Code:    "GROUP_PARSE_ERROR",
						Message: fmt.Sprintf("select 组为空：%s", gs.Name),
						Stage:   "compile",
						Snippet: gs.Raw,
					},
				}
			}
			out = append(out, model.Group{
				Name:    gs.Name,
				Type:    "select",
				Members: members,
			})
		case "url-test":
			members := make([]string, 0)
			for _, name := range allNames {
				if gs.Regex != nil && gs.Regex.MatchString(name) {
					members = append(members, name)
				}
			}
			if len(members) == 0 {
				return nil, &CompileError{
					AppError: model.AppError{
						Code:    "GROUP_PARSE_ERROR",
						Message: fmt.Sprintf("url-test 组匹配为空：%s", gs.Name),
						Stage:   "compile",
						Snippet: gs.Raw,
					},
				}
			}
			out = append(out, model.Group{
				Name:         gs.Name,
				Type:         "url-test",
				Members:      members,
				TestURL:      gs.TestURL,
				IntervalSec:  gs.IntervalSec,
				ToleranceMS:  gs.ToleranceMS,
				HasTolerance: gs.HasTolerance,
			})
		default:
			return nil, &CompileError{
				AppError: model.AppError{
					Code:    "GROUP_UNSUPPORTED_TYPE",
					Message: fmt.Sprintf("不支持的策略组类型：%s", gs.Type),
					Stage:   "compile",
					Snippet: gs.Raw,
				},
			}
		}
	}
	return out, nil
}

func compileRules(ctx context.Context, groupNameSet map[string]struct{}, prof *profile.Spec, expandRulesets bool) ([]model.Rule, []RulesetRef, error) {
	// Validate ruleset default actions.
	for _, rs := range prof.Ruleset {
		if rs.Action == "DIRECT" || rs.Action == "REJECT" {
			continue
		}
		if _, ok := groupNameSet[rs.Action]; !ok {
			return nil, nil, &CompileError{
				AppError: model.AppError{
					Code:    "REFERENCE_NOT_FOUND",
					Message: fmt.Sprintf("ruleset ACTION 引用不存在：%s", rs.Action),
					Stage:   "compile",
					Snippet: rs.Raw,
				},
			}
		}
	}

	rulesetRefs := make([]RulesetRef, 0, len(prof.Ruleset))
	for _, rs := range prof.Ruleset {
		rulesetRefs = append(rulesetRefs, RulesetRef{
			Raw:    rs.Raw,
			Action: rs.Action,
			URL:    rs.URL,
		})
	}

	out := make([]model.Rule, 0)
	if expandRulesets {
		// Expand rulesets first (SPEC_DETERMINISM.md).
		for _, rs := range prof.Ruleset {
			text, err := fetch.FetchText(ctx, fetch.KindRuleset, rs.URL)
			if err != nil {
				return nil, nil, err
			}
			ruleList, err := rules.ParseRulesetText(rs.URL, text, rs.Action)
			if err != nil {
				return nil, nil, err
			}
			out = append(out, ruleList...)
		}
	}

	// Then append inline rules (already parsed).
	out = append(out, prof.Rules...)

	// Validate: exactly one MATCH, and it must be the last rule.
	matchCount := 0
	matchIndex := -1
	for i, r := range out {
		if r.Type == "MATCH" {
			matchCount++
			matchIndex = i
		}
	}
	if matchCount != 1 {
		return nil, nil, &CompileError{
			AppError: model.AppError{
				Code:    "RULE_PARSE_ERROR",
				Message: fmt.Sprintf("兜底规则 MATCH 数量不合法（got=%d, want=1）", matchCount),
				Stage:   "compile",
			},
		}
	}
	if matchIndex != len(out)-1 {
		return nil, nil, &CompileError{
			AppError: model.AppError{
				Code:    "RULE_PARSE_ERROR",
				Message: "兜底规则 MATCH 必须是最后一条",
				Stage:   "compile",
			},
		}
	}

	// Validate action references.
	for _, r := range out {
		if r.Action == "DIRECT" || r.Action == "REJECT" {
			continue
		}
		if _, ok := groupNameSet[r.Action]; !ok {
			return nil, nil, &CompileError{
				AppError: model.AppError{
					Code:    "REFERENCE_NOT_FOUND",
					Message: fmt.Sprintf("规则 ACTION 引用不存在：%s", r.Action),
					Stage:   "compile",
					Snippet: ruleSnippet(r),
				},
			}
		}
	}

	return out, rulesetRefs, nil
}

func ruleSnippet(r model.Rule) string {
	if r.Type == "MATCH" {
		return fmt.Sprintf("MATCH,%s", r.Action)
	}
	if (r.Type == "IP-CIDR" || r.Type == "IP-CIDR6") && r.NoResolve {
		return fmt.Sprintf("%s,%s,%s,no-resolve", r.Type, r.Value, r.Action)
	}
	return fmt.Sprintf("%s,%s,%s", r.Type, r.Value, r.Action)
}
