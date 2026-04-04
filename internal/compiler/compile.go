package compiler

import (
	"errors"
	"fmt"
	"strings"

	"github.com/John-Robertt/subconverter-go/internal/model"
	"github.com/John-Robertt/subconverter-go/internal/profile"
)

type Result struct {
	Proxies     []model.Proxy
	Groups      []model.Group
	Rules       []model.Rule
	RulesetRefs []RulesetRef
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
// normalization + dedup + deterministic display naming + ordering.
func NormalizeSubscriptionProxies(subs []model.Proxy) ([]model.Proxy, error) {
	proxies, err := normalizeAndDedupSubscriptionProxies(subs)
	if err != nil {
		return nil, err
	}
	assignDisplayNames(proxies, map[string]struct{}{"DIRECT": {}, "REJECT": {}})
	return proxies, nil
}

func Compile(subs []model.Proxy, prof *profile.Spec) (*Result, error) {
	if prof == nil {
		return nil, &CompileError{AppError: model.AppError{Code: "PROFILE_VALIDATE_ERROR", Message: "profile 不能为空", Stage: "compile"}}
	}

	subProxies, err := normalizeAndDedupSubscriptionProxies(subs)
	if err != nil {
		return nil, err
	}
	if len(subProxies) == 0 {
		return nil, &CompileError{AppError: model.AppError{Code: "SUB_PARSE_ERROR", Message: "没有任何可用节点", Stage: "compile"}}
	}

	customProxies, err := compileCustomProxies(prof.CustomProxies)
	if err != nil {
		return nil, err
	}

	reservedNames, groupNameSet, err := validateFixedNameNamespace(customProxies, prof.Groups)
	if err != nil {
		return nil, err
	}

	assignDisplayNames(subProxies, reservedNames)

	groups, compiledGroups, err := compileGroups(subProxies, prof.Groups)
	if err != nil {
		return nil, err
	}

	usedCustomProxies, err := compileProxyChains(subProxies, customProxies, compiledGroups, prof.ProxyChains)
	if err != nil {
		return nil, err
	}

	rulesOut, rulesetRefs, err := compileRules(groupNameSet, prof)
	if err != nil {
		return nil, err
	}

	allProxies := make([]model.Proxy, 0, len(usedCustomProxies)+len(subProxies))
	allProxies = append(allProxies, usedCustomProxies...)
	allProxies = append(allProxies, subProxies...)

	return &Result{Proxies: allProxies, Groups: groups, Rules: rulesOut, RulesetRefs: rulesetRefs}, nil
}

type RulesetRef struct {
	Raw    string
	Action string
	URL    string
}

type groupMemberRef struct {
	Kind  string // proxy | group | builtin
	Value string
}

type compiledGroup struct {
	Name         string
	Type         string
	Members      []groupMemberRef
	TestURL      string
	IntervalSec  int
	ToleranceMS  int
	HasTolerance bool
}

func normalizeAndDedupSubscriptionProxies(in []model.Proxy) ([]model.Proxy, error) {
	normalized := make([]model.Proxy, 0, len(in))
	for _, p := range in {
		p2, err := normalizeSubscriptionProxy(p)
		if err != nil {
			return nil, &CompileError{
				AppError: model.AppError{Code: "SUB_PARSE_ERROR", Message: "节点字段不合法", Stage: "compile", Snippet: p.Name},
				Cause:    err,
			}
		}
		normalized = append(normalized, p2)
	}

	seen := make(map[string]struct{}, len(normalized))
	deduped := make([]model.Proxy, 0, len(normalized))
	for _, p := range normalized {
		key := dedupKey(p)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		p.Source = "subscription"
		p.MatchName = baseMatchName(p)
		deduped = append(deduped, p)
	}
	for i := range deduped {
		deduped[i].ID = fmt.Sprintf("sub:%d", i+1)
	}
	return deduped, nil
}

func compileCustomProxies(in []model.Proxy) ([]model.Proxy, error) {
	out := make([]model.Proxy, 0, len(in))
	for i, p := range in {
		p2, err := normalizeCustomProxy(p)
		if err != nil {
			return nil, &CompileError{
				AppError: model.AppError{Code: "CUSTOM_PROXY_VALIDATE_ERROR", Message: "custom_proxy 字段不合法", Stage: "compile", Snippet: p.Name},
				Cause:    err,
			}
		}
		p2.ID = fmt.Sprintf("custom:%d", i+1)
		p2.Source = "custom"
		p2.MatchName = p2.Name
		out = append(out, p2)
	}
	return out, nil
}

func validateFixedNameNamespace(customProxies []model.Proxy, groupSpecs []profile.GroupSpec) (map[string]struct{}, map[string]struct{}, error) {
	reserved := map[string]struct{}{"DIRECT": {}, "REJECT": {}}
	groupNameSet := make(map[string]struct{}, len(groupSpecs))

	for _, p := range customProxies {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			return nil, nil, &CompileError{AppError: model.AppError{Code: "CUSTOM_PROXY_VALIDATE_ERROR", Message: "custom_proxy.name 不能为空", Stage: "compile"}}
		}
		if name == "DIRECT" || name == "REJECT" {
			return nil, nil, &CompileError{AppError: model.AppError{Code: "CUSTOM_PROXY_VALIDATE_ERROR", Message: "custom_proxy.name 不能使用保留名 DIRECT/REJECT", Stage: "compile", Snippet: name}}
		}
		if _, ok := reserved[name]; ok {
			return nil, nil, &CompileError{AppError: model.AppError{Code: "CUSTOM_PROXY_VALIDATE_ERROR", Message: fmt.Sprintf("重复的 custom_proxy.name：%s", name), Stage: "compile", Snippet: name}}
		}
		reserved[name] = struct{}{}
	}

	for _, g := range groupSpecs {
		if g.Name == "" {
			return nil, nil, &CompileError{AppError: model.AppError{Code: "GROUP_PARSE_ERROR", Message: "策略组名不能为空", Stage: "compile", Snippet: g.Raw}}
		}
		if _, ok := groupNameSet[g.Name]; ok {
			return nil, nil, &CompileError{AppError: model.AppError{Code: "PROFILE_VALIDATE_ERROR", Message: fmt.Sprintf("重复的策略组名：%s", g.Name), Stage: "compile", Snippet: g.Raw}}
		}
		if _, ok := reserved[g.Name]; ok {
			return nil, nil, &CompileError{AppError: model.AppError{Code: "PROFILE_VALIDATE_ERROR", Message: fmt.Sprintf("策略组名与固定名称冲突：%s", g.Name), Stage: "compile", Snippet: g.Raw}}
		}
		groupNameSet[g.Name] = struct{}{}
		reserved[g.Name] = struct{}{}
	}

	return reserved, groupNameSet, nil
}

func assignDisplayNames(proxies []model.Proxy, reserved map[string]struct{}) {
	used := make(map[string]struct{}, len(reserved)+len(proxies))
	for k := range reserved {
		used[k] = struct{}{}
	}
	for i := range proxies {
		base := strings.TrimSpace(proxies[i].MatchName)
		if base == "" {
			base = baseMatchName(proxies[i])
		}
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
			for n := 2; ; n++ {
				try := fmt.Sprintf("%s-%d", base, n)
				if _, ok := used[try]; ok {
					continue
				}
				name = try
				break
			}
		}
		proxies[i].Name = name
		used[name] = struct{}{}
	}
}

func normalizeSubscriptionProxy(p model.Proxy) (model.Proxy, error) {
	if p.Type != "ss" {
		return model.Proxy{}, errors.New("only ss proxies are supported in v1 subscription input")
	}
	p.Name = strings.TrimSpace(p.Name)
	if strings.ContainsAny(p.Name, "\r\n\x00") {
		return model.Proxy{}, errors.New("proxy name contains control chars")
	}
	p.Server = strings.ToLower(strings.TrimSpace(p.Server))
	if p.Server == "" {
		return model.Proxy{}, errors.New("empty server")
	}
	p.Cipher = strings.ToLower(strings.TrimSpace(p.Cipher))
	if p.Cipher == "" {
		return model.Proxy{}, errors.New("empty cipher")
	}
	p.Password = strings.TrimSpace(p.Password)
	if p.Password == "" {
		return model.Proxy{}, errors.New("empty password")
	}
	p.Username = ""
	p.ViaProxyID = ""
	p.PluginName = strings.TrimSpace(p.PluginName)
	if len(p.PluginOpts) > 0 {
		opts := make([]model.KV, 0, len(p.PluginOpts))
		for _, kv := range p.PluginOpts {
			opts = append(opts, model.KV{Key: strings.TrimSpace(kv.Key), Value: strings.TrimSpace(kv.Value)})
		}
		p.PluginOpts = opts
	}
	return p, nil
}

func normalizeCustomProxy(p model.Proxy) (model.Proxy, error) {
	p.Name = strings.TrimSpace(p.Name)
	p.Type = strings.ToLower(strings.TrimSpace(p.Type))
	p.Server = strings.ToLower(strings.TrimSpace(p.Server))
	p.Username = strings.TrimSpace(p.Username)
	p.Password = strings.TrimSpace(p.Password)
	p.Cipher = strings.ToLower(strings.TrimSpace(p.Cipher))
	p.PluginName = strings.TrimSpace(p.PluginName)
	p.ViaProxyID = ""
	if p.Name == "" {
		return model.Proxy{}, errors.New("empty name")
	}
	if strings.ContainsAny(p.Name, "\r\n\x00") {
		return model.Proxy{}, errors.New("proxy name contains control chars")
	}
	if p.Server == "" {
		return model.Proxy{}, errors.New("empty server")
	}
	if p.Port < 1 || p.Port > 65535 {
		return model.Proxy{}, errors.New("invalid port")
	}
	if len(p.PluginOpts) > 0 {
		opts := make([]model.KV, 0, len(p.PluginOpts))
		for _, kv := range p.PluginOpts {
			opts = append(opts, model.KV{Key: strings.TrimSpace(kv.Key), Value: strings.TrimSpace(kv.Value)})
		}
		p.PluginOpts = opts
	}
	match := p.Name
	switch p.Type {
	case "ss":
		if p.Username != "" {
			return model.Proxy{}, errors.New("ss custom proxy does not support username")
		}
		if p.Cipher == "" || p.Password == "" {
			return model.Proxy{}, errors.New("ss custom proxy requires cipher/password")
		}
	case "http", "https", "socks5", "socks5-tls":
		if p.Cipher != "" || p.PluginName != "" || len(p.PluginOpts) > 0 {
			return model.Proxy{}, errors.New("non-ss custom proxy does not support ss-only fields")
		}
		if (p.Username == "") != (p.Password == "") {
			return model.Proxy{}, errors.New("username/password must be both set or both empty")
		}
	default:
		return model.Proxy{}, errors.New("unsupported custom proxy type")
	}
	p.MatchName = match
	return p, nil
}

func baseMatchName(p model.Proxy) string {
	base := strings.TrimSpace(p.Name)
	if base == "" {
		base = fmt.Sprintf("%s:%d", p.Server, p.Port)
	}
	return strings.ReplaceAll(base, "=", "-")
}

func dedupKey(p model.Proxy) string {
	var b strings.Builder
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

func compileGroups(proxies []model.Proxy, groupSpecs []profile.GroupSpec) ([]model.Group, map[string]*compiledGroup, error) {
	allRefs := make([]groupMemberRef, 0, len(proxies))
	allNames := make([]string, 0, len(proxies))
	for _, p := range proxies {
		allRefs = append(allRefs, groupMemberRef{Kind: "proxy", Value: p.ID})
		allNames = append(allNames, p.Name)
	}

	out := make([]model.Group, 0, len(groupSpecs))
	compiled := make(map[string]*compiledGroup, len(groupSpecs))
	for _, gs := range groupSpecs {
		switch gs.Type {
		case "select":
			members := make([]string, 0, len(gs.Members)+len(allNames))
			refs := make([]groupMemberRef, 0, len(gs.Members)+len(allRefs))
			if len(gs.Members) > 0 {
				for _, m := range gs.Members {
					switch m {
					case "@all":
						members = append(members, allNames...)
						refs = append(refs, allRefs...)
					case "DIRECT", "REJECT":
						members = append(members, m)
						refs = append(refs, groupMemberRef{Kind: "builtin", Value: m})
					default:
						members = append(members, m)
						refs = append(refs, groupMemberRef{Kind: "group", Value: m})
					}
				}
			} else if gs.Regex != nil {
				for _, p := range proxies {
					if gs.Regex.MatchString(p.MatchName) {
						members = append(members, p.Name)
						refs = append(refs, groupMemberRef{Kind: "proxy", Value: p.ID})
					}
				}
			}
			if len(members) == 0 {
				return nil, nil, &CompileError{AppError: model.AppError{Code: "GROUP_PARSE_ERROR", Message: fmt.Sprintf("select 组为空：%s", gs.Name), Stage: "compile", Snippet: gs.Raw}}
			}
			out = append(out, model.Group{Name: gs.Name, Type: "select", Members: members})
			compiled[gs.Name] = &compiledGroup{Name: gs.Name, Type: "select", Members: refs}
		case "url-test":
			members := make([]string, 0)
			refs := make([]groupMemberRef, 0)
			for _, p := range proxies {
				if gs.Regex != nil && gs.Regex.MatchString(p.MatchName) {
					members = append(members, p.Name)
					refs = append(refs, groupMemberRef{Kind: "proxy", Value: p.ID})
				}
			}
			if len(members) == 0 {
				return nil, nil, &CompileError{AppError: model.AppError{Code: "GROUP_PARSE_ERROR", Message: fmt.Sprintf("url-test 组匹配为空：%s", gs.Name), Stage: "compile", Snippet: gs.Raw}}
			}
			out = append(out, model.Group{Name: gs.Name, Type: "url-test", Members: members, TestURL: gs.TestURL, IntervalSec: gs.IntervalSec, ToleranceMS: gs.ToleranceMS, HasTolerance: gs.HasTolerance})
			compiled[gs.Name] = &compiledGroup{Name: gs.Name, Type: "url-test", Members: refs, TestURL: gs.TestURL, IntervalSec: gs.IntervalSec, ToleranceMS: gs.ToleranceMS, HasTolerance: gs.HasTolerance}
		default:
			return nil, nil, &CompileError{AppError: model.AppError{Code: "GROUP_UNSUPPORTED_TYPE", Message: fmt.Sprintf("不支持的策略组类型：%s", gs.Type), Stage: "compile", Snippet: gs.Raw}}
		}
	}
	return out, compiled, nil
}

func compileProxyChains(subs []model.Proxy, customs []model.Proxy, groups map[string]*compiledGroup, chains []profile.ChainSpec) ([]model.Proxy, error) {
	if len(chains) == 0 {
		for i := range subs {
			subs[i].ViaProxyID = ""
		}
		return nil, nil
	}

	customByName := make(map[string]model.Proxy, len(customs))
	for _, p := range customs {
		customByName[p.Name] = p
	}
	assigned := make(map[string]string, len(subs))
	usedCustom := make(map[string]struct{})
	for _, chain := range chains {
		via, ok := customByName[chain.Via]
		if !ok {
			return nil, &CompileError{AppError: model.AppError{Code: "CHAIN_VIA_NOT_FOUND", Message: fmt.Sprintf("proxy_chain via 引用不存在：%s", chain.Via), Stage: "compile", Snippet: chain.Raw}}
		}
		ids, err := selectChainProxyIDs(chain, subs, groups)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return nil, &CompileError{AppError: model.AppError{Code: "CHAIN_SELECTOR_EMPTY", Message: "proxy_chain 选择结果为空", Stage: "compile", Snippet: chain.Raw}}
		}
		for _, id := range ids {
			if prev, ok := assigned[id]; ok && prev != via.ID {
				return nil, &CompileError{AppError: model.AppError{Code: "CHAIN_CONFLICT", Message: "同一订阅节点命中了多个不同的链式出口", Stage: "compile", Snippet: chain.Raw, Hint: fmt.Sprintf("proxy_id=%s", id)}}
			}
			assigned[id] = via.ID
			usedCustom[via.ID] = struct{}{}
		}
	}
	for i := range subs {
		subs[i].ViaProxyID = assigned[subs[i].ID]
	}
	out := make([]model.Proxy, 0, len(usedCustom))
	for _, p := range customs {
		if _, ok := usedCustom[p.ID]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func selectChainProxyIDs(chain profile.ChainSpec, subs []model.Proxy, groups map[string]*compiledGroup) ([]string, error) {
	ids := make([]string, 0)
	switch chain.Type {
	case "all":
		for _, p := range subs {
			ids = append(ids, p.ID)
		}
	case "regex":
		for _, p := range subs {
			if chain.Regex != nil && chain.Regex.MatchString(p.MatchName) {
				ids = append(ids, p.ID)
			}
		}
	case "group":
		expanded, err := expandGroupProxyIDs(chain.Group, groups, make(map[string]bool))
		if err != nil {
			return nil, err
		}
		ids = append(ids, expanded...)
	default:
		return nil, &CompileError{AppError: model.AppError{Code: "CHAIN_PARSE_ERROR", Message: fmt.Sprintf("不支持的 proxy_chain.type：%s", chain.Type), Stage: "compile", Snippet: chain.Raw}}
	}
	return uniqueStrings(ids), nil
}

func expandGroupProxyIDs(name string, groups map[string]*compiledGroup, visiting map[string]bool) ([]string, error) {
	g, ok := groups[name]
	if !ok {
		return nil, &CompileError{AppError: model.AppError{Code: "CHAIN_GROUP_NOT_FOUND", Message: fmt.Sprintf("proxy_chain group 引用不存在：%s", name), Stage: "compile", Snippet: name}}
	}
	if visiting[name] {
		return nil, &CompileError{AppError: model.AppError{Code: "GROUP_REFERENCE_CYCLE", Message: fmt.Sprintf("策略组引用存在循环：%s", name), Stage: "compile", Snippet: name}}
	}
	visiting[name] = true
	defer delete(visiting, name)

	ids := make([]string, 0)
	for _, m := range g.Members {
		switch m.Kind {
		case "proxy":
			ids = append(ids, m.Value)
		case "group":
			nested, err := expandGroupProxyIDs(m.Value, groups, visiting)
			if err != nil {
				return nil, err
			}
			ids = append(ids, nested...)
		case "builtin":
			// Builtins are not subscription proxies; ignore them.
		}
	}
	return uniqueStrings(ids), nil
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func compileRules(groupNameSet map[string]struct{}, prof *profile.Spec) ([]model.Rule, []RulesetRef, error) {
	for _, rs := range prof.Ruleset {
		if rs.Action == "DIRECT" || rs.Action == "REJECT" {
			continue
		}
		if _, ok := groupNameSet[rs.Action]; !ok {
			return nil, nil, &CompileError{AppError: model.AppError{Code: "REFERENCE_NOT_FOUND", Message: fmt.Sprintf("ruleset ACTION 引用不存在：%s", rs.Action), Stage: "compile", Snippet: rs.Raw}}
		}
	}

	rulesetRefs := make([]RulesetRef, 0, len(prof.Ruleset))
	for _, rs := range prof.Ruleset {
		rulesetRefs = append(rulesetRefs, RulesetRef{Raw: rs.Raw, Action: rs.Action, URL: rs.URL})
	}

	out := prof.Rules
	matchCount := 0
	matchIndex := -1
	for i, r := range out {
		if r.Type == "MATCH" {
			matchCount++
			matchIndex = i
		}
	}
	if matchCount != 1 {
		return nil, nil, &CompileError{AppError: model.AppError{Code: "RULE_PARSE_ERROR", Message: fmt.Sprintf("兜底规则 MATCH 数量不合法（got=%d, want=1）", matchCount), Stage: "compile"}}
	}
	if matchIndex != len(out)-1 {
		return nil, nil, &CompileError{AppError: model.AppError{Code: "RULE_PARSE_ERROR", Message: "兜底规则 MATCH 必须是最后一条", Stage: "compile"}}
	}

	for _, r := range out {
		if r.Action == "DIRECT" || r.Action == "REJECT" {
			continue
		}
		if _, ok := groupNameSet[r.Action]; !ok {
			return nil, nil, &CompileError{AppError: model.AppError{Code: "REFERENCE_NOT_FOUND", Message: fmt.Sprintf("规则 ACTION 引用不存在：%s", r.Action), Stage: "compile", Snippet: ruleSnippet(r)}}
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
