package render

import (
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/John-Robertt/subconverter-go/internal/compiler"
	"github.com/John-Robertt/subconverter-go/internal/model"
)

func renderClash(res *compiler.Result) (Blocks, error) {
	proxyNameByID := make(map[string]string, len(res.Proxies))
	for _, p := range res.Proxies {
		proxyNameByID[p.ID] = p.Name
	}

	proxyLines := make([]string, 0, len(res.Proxies)*6)
	for _, p := range res.Proxies {
		lines, err := renderClashProxy(p, proxyNameByID)
		if err != nil {
			return Blocks{}, err
		}
		proxyLines = append(proxyLines, lines...)
	}

	groupLines := make([]string, 0, len(res.Groups)*6)
	for _, g := range res.Groups {
		groupLines = append(groupLines, "- name: "+yamlDQ(g.Name))
		groupLines = append(groupLines, "  type: "+yamlDQ(g.Type))
		groupLines = append(groupLines, "  proxies:")
		for _, m := range g.Members {
			groupLines = append(groupLines, "    - "+yamlDQ(m))
		}
		if g.Type == "url-test" {
			groupLines = append(groupLines, "  url: "+yamlDQ(g.TestURL))
			groupLines = append(groupLines, "  interval: "+strconv.Itoa(g.IntervalSec))
			if g.HasTolerance {
				groupLines = append(groupLines, "  tolerance: "+strconv.Itoa(g.ToleranceMS))
			}
		}
	}

	ruleProvidersBlock, providerNames, err := renderClashRuleProviders(res.RulesetRefs)
	if err != nil {
		return Blocks{}, err
	}

	ruleLines := make([]string, 0, len(res.RulesetRefs)+len(res.Rules))
	for i, rs := range res.RulesetRefs {
		ruleLines = append(ruleLines, "- "+yamlDQ("RULE-SET,"+providerNames[i]+","+rs.Action))
	}
	for _, r := range res.Rules {
		ruleLines = append(ruleLines, "- "+yamlDQ(ruleToClashString(r)))
	}

	return Blocks{
		Proxies:       strings.Join(proxyLines, "\n"),
		Groups:        strings.Join(groupLines, "\n"),
		RuleProviders: ruleProvidersBlock,
		Rules:         strings.Join(ruleLines, "\n"),
	}, nil
}

func renderClashProxy(p model.Proxy, proxyNameByID map[string]string) ([]string, error) {
	lines := []string{"- name: " + yamlDQ(p.Name)}
	switch p.Type {
	case "ss":
		lines = append(lines,
			"  type: ss",
			"  server: "+yamlDQ(p.Server),
			"  port: "+strconv.Itoa(p.Port),
			"  cipher: "+yamlDQ(strings.ToLower(p.Cipher)),
			"  password: "+yamlDQ(p.Password),
		)
		if p.PluginName != "" {
			plugin, mode, host, err := parseSSObfsPlugin(p)
			if err != nil {
				return nil, err
			}
			_ = plugin
			lines = append(lines, "  plugin: obfs", "  plugin-opts:", "    mode: "+yamlDQ(mode))
			if host != "" {
				lines = append(lines, "    host: "+yamlDQ(host))
			}
		}
	case "http", "https":
		lines = append(lines, "  type: http", "  server: "+yamlDQ(p.Server), "  port: "+strconv.Itoa(p.Port))
		if p.Type == "https" {
			lines = append(lines, "  tls: true")
		}
		if p.Username != "" || p.Password != "" {
			lines = append(lines, "  username: "+yamlDQ(p.Username), "  password: "+yamlDQ(p.Password))
		}
	case "socks5", "socks5-tls":
		lines = append(lines, "  type: socks5", "  server: "+yamlDQ(p.Server), "  port: "+strconv.Itoa(p.Port))
		if p.Type == "socks5-tls" {
			lines = append(lines, "  tls: true")
		}
		if p.Username != "" || p.Password != "" {
			lines = append(lines, "  username: "+yamlDQ(p.Username), "  password: "+yamlDQ(p.Password))
		}
	default:
		return nil, &RenderError{AppError: model.AppError{Code: "INVALID_ARGUMENT", Message: "不支持的代理类型渲染到 Clash", Stage: "render", Snippet: p.Type}}
	}

	if p.ViaProxyID != "" {
		viaName, ok := proxyNameByID[p.ViaProxyID]
		if !ok {
			return nil, &RenderError{AppError: model.AppError{Code: "CHAIN_VIA_NOT_FOUND", Message: "链式代理引用的出口不存在", Stage: "render", Snippet: p.ViaProxyID}}
		}
		lines = append(lines, "  dialer-proxy: "+yamlDQ(viaName))
	}
	return lines, nil
}

func yamlDQ(s string) string {
	// Minimal YAML double-quoted scalar escaping.
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return "\"" + s + "\""
}

func renderClashRuleProviders(refs []compiler.RulesetRef) (block string, providerNames []string, err error) {
	// Keep YAML valid even when the profile does not define any rulesets.
	if len(refs) == 0 {
		return "{}", nil, nil
	}

	used := make(map[string]int, len(refs))
	providerNames = make([]string, len(refs))
	lines := make([]string, 0, len(refs)*6)

	for i, rs := range refs {
		if strings.TrimSpace(rs.URL) == "" {
			return "", nil, &RenderError{
				AppError: model.AppError{
					Code:    "PROFILE_VALIDATE_ERROR",
					Message: "ruleset URL 不能为空",
					Stage:   "render",
					Snippet: rs.Raw,
				},
			}
		}
		if strings.ContainsAny(rs.URL, "\r\n\x00") {
			return "", nil, &RenderError{
				AppError: model.AppError{
					Code:    "PROFILE_VALIDATE_ERROR",
					Message: "ruleset URL 含有非法控制字符",
					Stage:   "render",
					Snippet: rs.URL,
				},
			}
		}

		name := clashRuleProviderName(rs.URL, used)
		providerNames[i] = name

		// Minimal provider config per https://wiki.metacubex.one/config/rule-providers/.
		lines = append(lines, name+":")
		lines = append(lines, "  type: http")
		lines = append(lines, "  behavior: classical")
		lines = append(lines, "  url: "+yamlDQ(rs.URL))
		lines = append(lines, "  interval: 86400")
		lines = append(lines, "  format: text")
	}

	return strings.Join(lines, "\n"), providerNames, nil
}

func clashRuleProviderName(rawURL string, used map[string]int) string {
	base := ""
	if u, err := url.Parse(strings.TrimSpace(rawURL)); err == nil && u != nil {
		base = path.Base(u.Path)
	}
	if base == "" || base == "." || base == "/" {
		base = "ruleset"
	}
	base = strings.TrimSuffix(base, path.Ext(base))
	base = sanitizeClashRuleProviderName(base)
	if base == "" {
		base = "ruleset"
	}

	if n, ok := used[base]; ok {
		n++
		used[base] = n
		return fmt.Sprintf("%s-%d", base, n)
	}
	used[base] = 1
	return base
}

func sanitizeClashRuleProviderName(s string) string {
	// Keep it in a safe subset so it can be used both as YAML key and in
	// "RULE-SET,providername,policy" without extra quoting rules.
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_-")
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}
