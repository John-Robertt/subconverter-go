package render

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/John-Robertt/subconverter-go/internal/compiler"
	"github.com/John-Robertt/subconverter-go/internal/model"
)

func renderQuanx(res *compiler.Result) (Blocks, error) {
	// Precompute representable proxy tags to keep references consistent.
	proxyTagRep := make(map[string]string, len(res.Proxies))
	for _, p := range res.Proxies {
		rep, err := quanxTag(p.Name)
		if err != nil {
			return Blocks{}, err
		}
		proxyTagRep[p.Name] = rep
	}

	proxyLines := make([]string, 0, len(res.Proxies))
	for _, p := range res.Proxies {
		if p.Type != "ss" {
			return Blocks{}, &RenderError{
				AppError: model.AppError{
					Code:    "INVALID_ARGUMENT",
					Message: "仅支持 ss 节点渲染",
					Stage:   "render",
					Snippet: p.Type,
				},
			}
		}

		tag := proxyTagRep[p.Name]
		line := fmt.Sprintf("shadowsocks = %s:%d, method=%s, password=%s, tag=%s", p.Server, p.Port, strings.ToLower(p.Cipher), p.Password, tag)
		if p.PluginName != "" {
			_, mode, host, err := parseSSObfsPlugin(p)
			if err != nil {
				return Blocks{}, err
			}
			line += ", obfs=" + mode
			if host != "" {
				line += ", obfs-host=" + host
			}
		}

		proxyLines = append(proxyLines, line)
	}

	groupLines := make([]string, 0, len(res.Groups))
	for _, g := range res.Groups {
		if err := quanxPolicyNameOK(g.Name); err != nil {
			return Blocks{}, err
		}
		switch g.Type {
		case "select":
			var b strings.Builder
			b.WriteString("static=")
			b.WriteString(g.Name)
			for _, m := range g.Members {
				b.WriteString(", ")
				b.WriteString(quanxMemberName(m, proxyTagRep))
			}
			groupLines = append(groupLines, b.String())
		case "url-test":
			var b strings.Builder
			b.WriteString("url-latency-benchmark=")
			b.WriteString(g.Name)
			for _, m := range g.Members {
				b.WriteString(", ")
				b.WriteString(quanxMemberName(m, proxyTagRep))
			}
			b.WriteString(", check-interval=")
			b.WriteString(strconv.Itoa(g.IntervalSec))
			if g.HasTolerance {
				b.WriteString(", tolerance=")
				b.WriteString(strconv.Itoa(g.ToleranceMS))
			}
			groupLines = append(groupLines, b.String())
		default:
			return Blocks{}, &RenderError{
				AppError: model.AppError{
					Code:    "INVALID_ARGUMENT",
					Message: fmt.Sprintf("不支持的策略组类型：%s", g.Type),
					Stage:   "render",
					Snippet: g.Type,
				},
			}
		}
	}

	rulesetLines := make([]string, 0, len(res.RulesetRefs))
	if len(res.RulesetRefs) > 0 {
		// Make tags unique even when multiple rulesets share the same action/policy.
		tagCounts := make(map[string]int, len(res.RulesetRefs))
		for _, rs := range res.RulesetRefs {
			if strings.ContainsAny(rs.URL, "\r\n\x00") || strings.Contains(rs.URL, ",") {
				return Blocks{}, &RenderError{
					AppError: model.AppError{
						Code:    "PROFILE_VALIDATE_ERROR",
						Message: "ruleset URL 含有 Quantumult X 不支持的字符（, 或控制字符）",
						Stage:   "render",
						Snippet: rs.URL,
						Hint:    "use a URL without ','",
					},
				}
			}

			policy, err := quanxActionName(rs.Action)
			if err != nil {
				return Blocks{}, err
			}

			tagBase := rs.Action
			tagCounts[tagBase]++
			tag := tagBase
			if tagCounts[tagBase] > 1 {
				tag = fmt.Sprintf("%s-%d", tagBase, tagCounts[tagBase])
			}
			if err := quanxPolicyNameOK(tag); err != nil {
				return Blocks{}, err
			}

			rulesetLines = append(rulesetLines, fmt.Sprintf("%s, tag=%s, force-policy=%s, enabled=true", rs.URL, tag, policy))
		}
	}

	ruleLines := make([]string, 0, len(res.Rules))
	for _, r := range res.Rules {
		action, err := quanxActionName(r.Action)
		if err != nil {
			return Blocks{}, err
		}
		ruleLines = append(ruleLines, ruleToQuanxString(r, action))
	}

	return Blocks{
		Proxies: strings.Join(proxyLines, "\n"),
		Groups:  strings.Join(groupLines, "\n"),
		Rulesets: strings.Join(rulesetLines, "\n"),
		Rules:   strings.Join(ruleLines, "\n"),
	}, nil
}

func quanxTag(tag string) (string, error) {
	if strings.ContainsAny(tag, "\r\n\x00") {
		return "", &RenderError{
			AppError: model.AppError{
				Code:    "SUB_PARSE_ERROR",
				Message: "节点名包含非法控制字符",
				Stage:   "render",
				Snippet: tag,
			},
		}
	}
	if strings.Contains(tag, "\"") {
		return "", &RenderError{
			AppError: model.AppError{
				Code:    "SUB_PARSE_ERROR",
				Message: "节点名包含双引号，无法输出到 Quantumult X",
				Stage:   "render",
				Snippet: tag,
				Hint:    "remove '\"' from node name",
			},
		}
	}
	// Quote commas to avoid breaking the comma-separated syntax.
	if strings.Contains(tag, ",") {
		return "\"" + tag + "\"", nil
	}
	return tag, nil
}

func quanxPolicyNameOK(name string) error {
	if strings.ContainsAny(name, "\r\n\x00") || strings.Contains(name, ",") || strings.Contains(name, "=") {
		return &RenderError{
			AppError: model.AppError{
				Code:    "PROFILE_VALIDATE_ERROR",
				Message: "策略组名/规则 action 含有 Quantumult X 不支持的字符（, 或 = 或控制字符）",
				Stage:   "render",
				Snippet: name,
				Hint:    "rename the group/action in profile",
			},
		}
	}
	return nil
}

func quanxMemberName(member string, proxyTagRep map[string]string) string {
	switch member {
	case "DIRECT":
		return "direct"
	case "REJECT":
		return "reject"
	}
	if rep, ok := proxyTagRep[member]; ok {
		return rep
	}
	return member
}

func quanxActionName(action string) (string, error) {
	switch action {
	case "DIRECT":
		return "direct", nil
	case "REJECT":
		return "reject", nil
	}
	if err := quanxPolicyNameOK(action); err != nil {
		return "", err
	}
	return action, nil
}

func ruleToQuanxString(r model.Rule, action string) string {
	typ := r.Type
	switch typ {
	case "MATCH":
		return fmt.Sprintf("FINAL,%s", action)
	case "IP-CIDR6":
		typ = "IP6-CIDR"
	}

	if (typ == "IP-CIDR" || typ == "IP6-CIDR") && r.NoResolve {
		return fmt.Sprintf("%s,%s,%s,no-resolve", typ, r.Value, action)
	}
	return fmt.Sprintf("%s,%s,%s", typ, r.Value, action)
}
