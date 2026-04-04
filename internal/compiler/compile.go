package compiler

import (
	"crypto/sha256"
	"encoding/hex"
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
// normalization + dedup + deterministic naming + ordering.
func NormalizeSubscriptionProxies(subs []model.Proxy) ([]model.Proxy, error) {
	return compileSubscriptionProxies(subs)
}

func Compile(subs []model.Proxy, prof *profile.Spec) (*Result, error) {
	if prof == nil {
		return nil, &CompileError{AppError: model.AppError{
			Code:    "PROFILE_VALIDATE_ERROR",
			Message: "profile 不能为空",
			Stage:   "compile",
		}}
	}

	subProxies, err := compileSubscriptionProxies(subs)
	if err != nil {
		return nil, err
	}
	if len(subProxies) == 0 {
		return nil, &CompileError{AppError: model.AppError{
			Code:    "SUB_PARSE_ERROR",
			Message: "没有任何可用节点",
			Stage:   "compile",
		}}
	}

	userGroupNameSet := make(map[string]struct{}, len(prof.Groups))
	for _, g := range prof.Groups {
		userGroupNameSet[g.Name] = struct{}{}
	}
	if err := validateGroupProxyNamespace(userGroupNameSet, subProxies); err != nil {
		return nil, err
	}

	customProxies, err := compileCustomProxies(prof.CustomProxies)
	if err != nil {
		return nil, err
	}

	preGroups, err := compileGroups(subProxies, subProxies, prof.Groups, true)
	if err != nil {
		return nil, err
	}

	derivedProxies, derivedByCustom, autoGroupNameSet, err := compileDerivedProxies(subProxies, customProxies, preGroups, prof.ProxyChains, userGroupNameSet)
	if err != nil {
		return nil, err
	}
	if err := validateGroupProxyNamespace(autoGroupNameSet, subProxies); err != nil {
		return nil, err
	}

	allProxies := make([]model.Proxy, 0, len(subProxies)+len(derivedProxies))
	allProxies = append(allProxies, subProxies...)
	allProxies = append(allProxies, derivedProxies...)

	userGroups, err := compileGroups(allProxies, subProxies, prof.Groups, false)
	if err != nil {
		return nil, err
	}
	autoGroups := buildDiagnosticGroups(customProxies, derivedByCustom)
	groups := append(userGroups, autoGroups...)

	groupNameSet := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		groupNameSet[g.Name] = struct{}{}
	}

	rulesOut, rulesetRefs, err := compileRules(groupNameSet, prof)
	if err != nil {
		return nil, err
	}

	return &Result{
		Proxies:     allProxies,
		Groups:      groups,
		Rules:       rulesOut,
		RulesetRefs: rulesetRefs,
	}, nil
}

type RulesetRef struct {
	Raw    string
	Action string
	URL    string
}

func compileSubscriptionProxies(in []model.Proxy) ([]model.Proxy, error) {
	normalized := make([]model.Proxy, 0, len(in))
	for _, p := range in {
		p2, err := normalizeSubscriptionProxy(p)
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

	seen := make(map[string]struct{}, len(normalized))
	deduped := make([]model.Proxy, 0, len(normalized))
	for _, p := range normalized {
		key := dedupKey(p)
		p.ID = proxyIDFromKey(key)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, p)
	}

	used := make(map[string]struct{}, len(deduped))
	for i := range deduped {
		base := baseSubscriptionName(deduped[i])
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
		deduped[i].Name = name
		used[name] = struct{}{}
	}

	return deduped, nil
}

func compileCustomProxies(in []model.Proxy) ([]model.Proxy, error) {
	out := make([]model.Proxy, 0, len(in))
	for _, p := range in {
		p2, err := normalizeCustomProxy(p)
		if err != nil {
			return nil, &CompileError{
				AppError: model.AppError{
					Code:    "CUSTOM_PROXY_VALIDATE_ERROR",
					Message: "custom_proxy 字段不合法",
					Stage:   "compile",
					Snippet: p.Name,
				},
				Cause: err,
			}
		}
		out = append(out, p2)
	}
	return out, nil
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
	if p.Port < 1 || p.Port > 65535 {
		return model.Proxy{}, errors.New("invalid port")
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
	p.Cipher = strings.ToLower(strings.TrimSpace(p.Cipher))
	p.Password = strings.TrimSpace(p.Password)
	p.PluginName = strings.TrimSpace(p.PluginName)
	p.ViaProxyID = ""
	if p.Name == "" {
		return model.Proxy{}, errors.New("empty name")
	}
	if strings.ContainsAny(p.Name, "\r\n\x00") {
		return model.Proxy{}, errors.New("custom proxy name contains control chars")
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

	return p, nil
}

func baseSubscriptionName(p model.Proxy) string {
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

func proxyIDFromKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func customProxyKey(p model.Proxy) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(strings.TrimSpace(p.Type)))
	b.WriteByte('\n')
	b.WriteString(strings.TrimSpace(p.Name))
	b.WriteByte('\n')
	b.WriteString(strings.ToLower(strings.TrimSpace(p.Server)))
	b.WriteByte('\n')
	b.WriteString(fmt.Sprintf("%d", p.Port))
	b.WriteByte('\n')
	b.WriteString(strings.TrimSpace(p.Username))
	b.WriteByte('\n')
	b.WriteString(strings.ToLower(strings.TrimSpace(p.Cipher)))
	b.WriteByte('\n')
	b.WriteString(strings.TrimSpace(p.Password))
	b.WriteByte('\n')
	b.WriteString(strings.TrimSpace(p.PluginName))
	b.WriteByte('\n')
	for _, kv := range p.PluginOpts {
		b.WriteString(kv.Key)
		b.WriteByte('=')
		b.WriteString(kv.Value)
		b.WriteByte(';')
	}
	return b.String()
}

func derivedProxyID(p model.Proxy, viaProxyID string) string {
	return proxyIDFromKey(customProxyKey(p) + "\n" + viaProxyID)
}

func compileGroups(matchProxies []model.Proxy, allProxyRefs []model.Proxy, groupSpecs []profile.GroupSpec, allowEmpty bool) ([]model.Group, error) {
	out := make([]model.Group, 0, len(groupSpecs))
	for _, gs := range groupSpecs {
		switch gs.Type {
		case "select":
			var members []model.MemberRef
			if len(gs.Members) > 0 {
				members = make([]model.MemberRef, 0, len(gs.Members)+len(allProxyRefs))
				for _, m := range gs.Members {
					if m == "@all" {
						members = append(members, allProxyMemberRefs(allProxyRefs)...)
						continue
					}
					members = append(members, explicitMemberRef(m))
				}
			} else if gs.Regex != nil {
				members = make([]model.MemberRef, 0)
				for _, p := range matchProxies {
					if gs.Regex.MatchString(p.Name) {
						members = append(members, proxyMemberRef(p.ID))
					}
				}
			}
			if len(members) == 0 && !allowEmpty {
				return nil, &CompileError{AppError: model.AppError{
					Code:    "GROUP_PARSE_ERROR",
					Message: fmt.Sprintf("select 组为空：%s", gs.Name),
					Stage:   "compile",
					Snippet: gs.Raw,
				}}
			}
			out = append(out, model.Group{Name: gs.Name, Type: "select", Members: members})
		case "url-test":
			members := make([]model.MemberRef, 0)
			for _, p := range matchProxies {
				if gs.Regex != nil && gs.Regex.MatchString(p.Name) {
					members = append(members, proxyMemberRef(p.ID))
				}
			}
			if len(members) == 0 && !allowEmpty {
				return nil, &CompileError{AppError: model.AppError{
					Code:    "GROUP_PARSE_ERROR",
					Message: fmt.Sprintf("url-test 组匹配为空：%s", gs.Name),
					Stage:   "compile",
					Snippet: gs.Raw,
				}}
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
			return nil, &CompileError{AppError: model.AppError{
				Code:    "GROUP_UNSUPPORTED_TYPE",
				Message: fmt.Sprintf("不支持的策略组类型：%s", gs.Type),
				Stage:   "compile",
				Snippet: gs.Raw,
			}}
		}
	}
	return out, nil
}

func allProxyMemberRefs(proxies []model.Proxy) []model.MemberRef {
	refs := make([]model.MemberRef, 0, len(proxies))
	for _, p := range proxies {
		refs = append(refs, proxyMemberRef(p.ID))
	}
	return refs
}

func proxyMemberRef(id string) model.MemberRef {
	return model.MemberRef{Kind: model.MemberRefProxy, Value: id}
}

func explicitMemberRef(raw string) model.MemberRef {
	if raw == "DIRECT" || raw == "REJECT" {
		return model.MemberRef{Kind: model.MemberRefBuiltin, Value: raw}
	}
	return model.MemberRef{Kind: model.MemberRefGroup, Value: raw}
}

func compileDerivedProxies(subs []model.Proxy, customs []model.Proxy, preGroups []model.Group, chains []profile.ChainSpec, userGroupNames map[string]struct{}) ([]model.Proxy, map[string][]model.Proxy, map[string]struct{}, error) {
	if len(chains) == 0 || len(customs) == 0 {
		return nil, map[string][]model.Proxy{}, map[string]struct{}{}, nil
	}

	customByName := make(map[string]model.Proxy, len(customs))
	for _, p := range customs {
		customByName[p.Name] = p
	}
	preGroupByName := make(map[string]model.Group, len(preGroups))
	for _, g := range preGroups {
		preGroupByName[g.Name] = g
	}

	selectedByCustom := make(map[string]map[string]struct{}, len(customs))
	for _, chain := range chains {
		customProxy, ok := customByName[chain.Proxy]
		if !ok {
			return nil, nil, nil, &CompileError{AppError: model.AppError{
				Code:    "CHAIN_PROXY_NOT_FOUND",
				Message: fmt.Sprintf("proxy_chain proxy 引用不存在：%s", chain.Proxy),
				Stage:   "compile",
				Snippet: chain.Raw,
			}}
		}
		ids, err := selectChainProxyIDs(chain, subs, preGroupByName)
		if err != nil {
			return nil, nil, nil, err
		}
		if len(ids) == 0 {
			return nil, nil, nil, &CompileError{AppError: model.AppError{
				Code:    "CHAIN_SELECTOR_EMPTY",
				Message: "proxy_chain 选择结果为空",
				Stage:   "compile",
				Snippet: chain.Raw,
			}}
		}
		selected := selectedByCustom[customProxy.Name]
		if selected == nil {
			selected = make(map[string]struct{}, len(ids))
			selectedByCustom[customProxy.Name] = selected
		}
		for _, id := range ids {
			selected[id] = struct{}{}
		}
	}

	autoGroupNames := make(map[string]struct{})
	for _, custom := range customs {
		if len(selectedByCustom[custom.Name]) == 0 {
			continue
		}
		autoGroupNames[autoDiagnosticGroupName(custom.Name)] = struct{}{}
	}

	usedNames := make(map[string]struct{}, len(subs)+len(userGroupNames)+len(autoGroupNames))
	for _, sub := range subs {
		if _, ok := autoGroupNames[sub.Name]; ok {
			return nil, nil, nil, &CompileError{AppError: model.AppError{
				Code:    "PROFILE_VALIDATE_ERROR",
				Message: fmt.Sprintf("策略组名与节点名冲突：%s", sub.Name),
				Stage:   "compile",
			}}
		}
		usedNames[sub.Name] = struct{}{}
	}
	for name := range userGroupNames {
		usedNames[name] = struct{}{}
	}
	for name := range autoGroupNames {
		usedNames[name] = struct{}{}
	}

	out := make([]model.Proxy, 0)
	derivedByCustom := make(map[string][]model.Proxy, len(customs))
	for _, custom := range customs {
		selected := selectedByCustom[custom.Name]
		if len(selected) == 0 {
			continue
		}
		derivedForCustom := make([]model.Proxy, 0, len(selected))
		for _, sub := range subs {
			if _, ok := selected[sub.ID]; !ok {
				continue
			}
			derived := custom
			derived.ID = derivedProxyID(custom, sub.ID)
			derived.ViaProxyID = sub.ID
			derived.Name = nextAvailableName(fmt.Sprintf("%s via %s", custom.Name, sub.Name), usedNames)
			out = append(out, derived)
			derivedForCustom = append(derivedForCustom, derived)
		}
		derivedByCustom[custom.Name] = derivedForCustom
	}

	return out, derivedByCustom, autoGroupNames, nil
}

func selectChainProxyIDs(chain profile.ChainSpec, subs []model.Proxy, groups map[string]model.Group) ([]string, error) {
	ids := make([]string, 0)
	switch chain.Type {
	case "all":
		for _, p := range subs {
			ids = append(ids, p.ID)
		}
	case "regex":
		for _, p := range subs {
			if chain.Regex != nil && chain.Regex.MatchString(p.Name) {
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
		return nil, &CompileError{AppError: model.AppError{
			Code:    "CHAIN_PARSE_ERROR",
			Message: fmt.Sprintf("不支持的 proxy_chain.type：%s", chain.Type),
			Stage:   "compile",
			Snippet: chain.Raw,
		}}
	}
	return uniqueStrings(ids), nil
}

func expandGroupProxyIDs(name string, groups map[string]model.Group, visiting map[string]bool) ([]string, error) {
	g, ok := groups[name]
	if !ok {
		return nil, &CompileError{AppError: model.AppError{
			Code:    "CHAIN_GROUP_NOT_FOUND",
			Message: fmt.Sprintf("proxy_chain group 引用不存在：%s", name),
			Stage:   "compile",
			Snippet: name,
		}}
	}
	if visiting[name] {
		return nil, &CompileError{AppError: model.AppError{
			Code:    "GROUP_REFERENCE_CYCLE",
			Message: fmt.Sprintf("策略组引用存在循环：%s", name),
			Stage:   "compile",
			Snippet: name,
		}}
	}
	visiting[name] = true
	defer delete(visiting, name)

	ids := make([]string, 0)
	for _, m := range g.Members {
		switch m.Kind {
		case model.MemberRefProxy:
			ids = append(ids, m.Value)
		case model.MemberRefGroup:
			nested, err := expandGroupProxyIDs(m.Value, groups, visiting)
			if err != nil {
				return nil, err
			}
			ids = append(ids, nested...)
		case model.MemberRefBuiltin:
			// Builtins are not subscription proxies.
		default:
			return nil, &CompileError{AppError: model.AppError{
				Code:    "GROUP_PARSE_ERROR",
				Message: "策略组成员引用类型不合法",
				Stage:   "compile",
				Snippet: m.Kind + ":" + m.Value,
			}}
		}
	}
	return uniqueStrings(ids), nil
}

func buildDiagnosticGroups(customs []model.Proxy, derivedByCustom map[string][]model.Proxy) []model.Group {
	out := make([]model.Group, 0, len(derivedByCustom))
	for _, custom := range customs {
		derived := derivedByCustom[custom.Name]
		if len(derived) == 0 {
			continue
		}
		members := make([]model.MemberRef, 0, len(derived))
		for _, p := range derived {
			members = append(members, proxyMemberRef(p.ID))
		}
		out = append(out, model.Group{
			Name:    autoDiagnosticGroupName(custom.Name),
			Type:    "select",
			Members: members,
		})
	}
	return out
}

func autoDiagnosticGroupName(customProxyName string) string {
	return "CHAIN-" + customProxyName
}

func validateGroupProxyNamespace(groupNames map[string]struct{}, proxies []model.Proxy) error {
	if len(groupNames) == 0 || len(proxies) == 0 {
		return nil
	}
	for _, p := range proxies {
		if _, ok := groupNames[p.Name]; ok {
			return &CompileError{AppError: model.AppError{
				Code:    "PROFILE_VALIDATE_ERROR",
				Message: fmt.Sprintf("策略组名与节点名冲突：%s", p.Name),
				Stage:   "compile",
			}}
		}
	}
	return nil
}

func nextAvailableName(base string, used map[string]struct{}) string {
	base = strings.TrimSpace(base)
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
	used[name] = struct{}{}
	return name
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
			return nil, nil, &CompileError{AppError: model.AppError{
				Code:    "REFERENCE_NOT_FOUND",
				Message: fmt.Sprintf("ruleset ACTION 引用不存在：%s", rs.Action),
				Stage:   "compile",
				Snippet: rs.Raw,
			}}
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
		return nil, nil, &CompileError{AppError: model.AppError{
			Code:    "RULE_PARSE_ERROR",
			Message: fmt.Sprintf("兜底规则 MATCH 数量不合法（got=%d, want=1）", matchCount),
			Stage:   "compile",
		}}
	}
	if matchIndex != len(out)-1 {
		return nil, nil, &CompileError{AppError: model.AppError{
			Code:    "RULE_PARSE_ERROR",
			Message: "兜底规则 MATCH 必须是最后一条",
			Stage:   "compile",
		}}
	}

	for _, r := range out {
		if r.Action == "DIRECT" || r.Action == "REJECT" {
			continue
		}
		if _, ok := groupNameSet[r.Action]; !ok {
			return nil, nil, &CompileError{AppError: model.AppError{
				Code:    "REFERENCE_NOT_FOUND",
				Message: fmt.Sprintf("规则 ACTION 引用不存在：%s", r.Action),
				Stage:   "compile",
				Snippet: ruleSnippet(r),
			}}
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
