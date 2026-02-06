package render

import (
	"strconv"
	"strings"

	"github.com/John-Robertt/subconverter-go/internal/compiler"
	"github.com/John-Robertt/subconverter-go/internal/model"
)

func renderClash(res *compiler.Result) (Blocks, error) {
	proxyLines := make([]string, 0, len(res.Proxies)*6)
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

		proxyLines = append(proxyLines, "- name: "+yamlDQ(p.Name))
		proxyLines = append(proxyLines, "  type: ss")
		proxyLines = append(proxyLines, "  server: "+yamlDQ(p.Server))
		proxyLines = append(proxyLines, "  port: "+strconv.Itoa(p.Port))
		proxyLines = append(proxyLines, "  cipher: "+yamlDQ(strings.ToLower(p.Cipher)))
		// Always quote password to avoid YAML treating it as number.
		proxyLines = append(proxyLines, "  password: "+yamlDQ(p.Password))

		if p.PluginName != "" {
			plugin, mode, host, err := parseSSObfsPlugin(p)
			if err != nil {
				return Blocks{}, err
			}
			_ = plugin // currently only "obfs"
			proxyLines = append(proxyLines, "  plugin: obfs")
			proxyLines = append(proxyLines, "  plugin-opts:")
			proxyLines = append(proxyLines, "    mode: "+yamlDQ(mode))
			if host != "" {
				proxyLines = append(proxyLines, "    host: "+yamlDQ(host))
			}
		}
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

	ruleLines := make([]string, 0, len(res.Rules))
	for _, r := range res.Rules {
		ruleLines = append(ruleLines, "- "+yamlDQ(ruleToClashString(r)))
	}

	return Blocks{
		Proxies: strings.Join(proxyLines, "\n"),
		Groups:  strings.Join(groupLines, "\n"),
		Rules:   strings.Join(ruleLines, "\n"),
	}, nil
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
