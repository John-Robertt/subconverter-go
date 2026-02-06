package render

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/John-Robertt/subconverter-go/internal/compiler"
	"github.com/John-Robertt/subconverter-go/internal/model"
)

func renderSurgeLike(res *compiler.Result, isSurge bool) (Blocks, error) {
	proxyLines := make([]string, 0, len(res.Proxies)+2)
	// Built-ins required by spec.
	proxyLines = append(proxyLines, "DIRECT = direct")
	proxyLines = append(proxyLines, "REJECT = reject")

	// Precompute name representation for proxies to keep references consistent.
	proxyNameRep := make(map[string]string, len(res.Proxies))
	for _, p := range res.Proxies {
		rep, err := surgeProxyName(p.Name)
		if err != nil {
			return Blocks{}, err
		}
		proxyNameRep[p.Name] = rep
	}

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

		name := proxyNameRep[p.Name]
		line := fmt.Sprintf("%s = ss, %s, %d, encrypt-method=%s, password=%s", name, p.Server, p.Port, strings.ToLower(p.Cipher), p.Password)

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
		if err := surgeGroupNameOK(g.Name); err != nil {
			return Blocks{}, err
		}
		switch g.Type {
		case "select":
			var b strings.Builder
			b.WriteString(g.Name)
			b.WriteString(" = select")
			for _, m := range g.Members {
				b.WriteString(", ")
				b.WriteString(surgeMemberName(m, proxyNameRep))
			}
			groupLines = append(groupLines, b.String())
		case "url-test":
			var b strings.Builder
			b.WriteString(g.Name)
			b.WriteString(" = url-test")
			for _, m := range g.Members {
				b.WriteString(", ")
				b.WriteString(surgeMemberName(m, proxyNameRep))
			}
			b.WriteString(", url=")
			b.WriteString(g.TestURL)
			b.WriteString(", interval=")
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

	ruleLines := make([]string, 0, len(res.Rules))

	// ruleset: render as remote references (RULE-SET) instead of expanding into
	// individual rule lines. This keeps the output small and lets clients fetch
	// rulesets directly.
	for _, rs := range res.RulesetRefs {
		if rs.URL == "" {
			return Blocks{}, &RenderError{
				AppError: model.AppError{
					Code:    "PROFILE_VALIDATE_ERROR",
					Message: "ruleset URL 不能为空",
					Stage:   "render",
					Snippet: rs.Raw,
				},
			}
		}
		if strings.ContainsAny(rs.URL, "\r\n\x00") || strings.Contains(rs.URL, ",") {
			return Blocks{}, &RenderError{
				AppError: model.AppError{
					Code:    "PROFILE_VALIDATE_ERROR",
					Message: "ruleset URL 含有 Surge/Shadowrocket 不支持的字符（, 或控制字符）",
					Stage:   "render",
					Snippet: rs.URL,
					Hint:    "use a URL without ','",
				},
			}
		}
		if rs.Action != "DIRECT" && rs.Action != "REJECT" {
			if err := surgeGroupNameOK(rs.Action); err != nil {
				return Blocks{}, err
			}
		}
		ruleLines = append(ruleLines, "RULE-SET,"+rs.URL+","+rs.Action)
	}

	for _, r := range res.Rules {
		// Validate action representability for Surge-like formats.
		if r.Action != "DIRECT" && r.Action != "REJECT" {
			if err := surgeGroupNameOK(r.Action); err != nil {
				return Blocks{}, err
			}
		}
		ruleLines = append(ruleLines, ruleToSurgeString(r))
	}

	_ = isSurge // stage7 handles managed-config line
	return Blocks{
		Proxies: strings.Join(proxyLines, "\n"),
		Groups:  strings.Join(groupLines, "\n"),
		Rules:   strings.Join(ruleLines, "\n"),
	}, nil
}

func surgeProxyName(name string) (string, error) {
	if strings.ContainsAny(name, "\r\n\x00") {
		return "", &RenderError{
			AppError: model.AppError{
				Code:    "SUB_PARSE_ERROR",
				Message: "节点名包含非法控制字符",
				Stage:   "render",
				Snippet: name,
			},
		}
	}
	if strings.Contains(name, "\"") {
		return "", &RenderError{
			AppError: model.AppError{
				Code:    "SUB_PARSE_ERROR",
				Message: "节点名包含双引号，无法输出到 Surge/Shadowrocket",
				Stage:   "render",
				Snippet: name,
				Hint:    "remove '\"' from node name",
			},
		}
	}
	if strings.Contains(name, "=") {
		return "", &RenderError{
			AppError: model.AppError{
				Code:    "SUB_PARSE_ERROR",
				Message: "节点名包含 '='，无法输出到 Surge/Shadowrocket",
				Stage:   "render",
				Snippet: name,
			},
		}
	}
	if strings.Contains(name, ",") {
		return "\"" + name + "\"", nil
	}
	return name, nil
}

func surgeGroupNameOK(name string) error {
	if strings.ContainsAny(name, "\r\n\x00") || strings.Contains(name, ",") || strings.Contains(name, "=") {
		return &RenderError{
			AppError: model.AppError{
				Code:    "PROFILE_VALIDATE_ERROR",
				Message: "策略组名/规则 action 含有 Surge 不支持的字符（, 或 = 或控制字符）",
				Stage:   "render",
				Snippet: name,
				Hint:    "rename the group/action in profile",
			},
		}
	}
	return nil
}

func surgeMemberName(member string, proxyNameRep map[string]string) string {
	// If it's a proxy name, use its representable form.
	if rep, ok := proxyNameRep[member]; ok {
		return rep
	}
	// Otherwise it's DIRECT/REJECT or group name, keep as-is.
	return member
}
