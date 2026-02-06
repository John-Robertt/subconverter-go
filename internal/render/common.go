package render

import (
	"fmt"
	"strings"

	"github.com/John-Robertt/subconverter-go/internal/model"
)

func ruleToClashString(r model.Rule) string {
	if r.Type == "MATCH" {
		return fmt.Sprintf("MATCH,%s", r.Action)
	}
	if r.Type == "IP-CIDR" && r.NoResolve {
		return fmt.Sprintf("%s,%s,%s,no-resolve", r.Type, r.Value, r.Action)
	}
	return fmt.Sprintf("%s,%s,%s", r.Type, r.Value, r.Action)
}

func ruleToSurgeString(r model.Rule) string {
	typ := r.Type
	if typ == "MATCH" {
		typ = "FINAL"
		return fmt.Sprintf("%s,%s", typ, r.Action)
	}
	if r.Type == "IP-CIDR" && r.NoResolve {
		return fmt.Sprintf("%s,%s,%s,no-resolve", typ, r.Value, r.Action)
	}
	return fmt.Sprintf("%s,%s,%s", typ, r.Value, r.Action)
}

// parseSSObfsPlugin validates and extracts the v1 supported ss plugin.
//
// Supported: simple-obfs / obfs-local
// Required option: obfs=<mode>
// Optional: obfs-host=<host>
func parseSSObfsPlugin(p model.Proxy) (plugin string, mode string, host string, err error) {
	if p.PluginName == "" {
		return "", "", "", nil
	}
	if p.PluginName != "simple-obfs" && p.PluginName != "obfs-local" {
		return "", "", "", &RenderError{
			AppError: model.AppError{
				Code:    "UNSUPPORTED_PLUGIN",
				Message: fmt.Sprintf("不支持的 SS plugin：%s", p.PluginName),
				Stage:   "render",
				Snippet: p.PluginName,
			},
		}
	}

	var obfsMode string
	var obfsHost string
	for _, kv := range p.PluginOpts {
		switch strings.TrimSpace(kv.Key) {
		case "obfs":
			obfsMode = strings.TrimSpace(kv.Value)
		case "obfs-host":
			obfsHost = strings.TrimSpace(kv.Value)
		}
	}
	if obfsMode == "" {
		return "", "", "", &RenderError{
			AppError: model.AppError{
				Code:    "UNSUPPORTED_PLUGIN",
				Message: "simple-obfs/obfs-local 缺少必需选项 obfs=<mode>",
				Stage:   "render",
				Snippet: p.PluginName,
				Hint:    "example: ?plugin=simple-obfs;obfs=tls;obfs-host=example.com",
			},
		}
	}

	return "obfs", obfsMode, obfsHost, nil
}
