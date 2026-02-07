package template

import (
	"testing"

	"github.com/John-Robertt/subconverter-go/internal/render"
)

func FuzzInjectAnchors_Clash(f *testing.F) {
	tmpl := "proxies:\n  #@PROXIES@#\nproxy-groups:\n  #@GROUPS@#\nrule-providers:\n  #@RULE_PROVIDERS@#\nrules:\n  #@RULES@#\n"
	tmplCRLF := "proxies:\r\n  #@PROXIES@#\r\nproxy-groups:\r\n  #@GROUPS@#\r\nrule-providers:\r\n  #@RULE_PROVIDERS@#\r\nrules:\r\n  #@RULES@#\r\n"

	proxies := "- name: \"A\"\n  type: ss\n  server: \"example.com\"\n  port: 8388\n  cipher: \"aes-128-gcm\"\n  password: \"pass\"\n"
	groups := "- name: \"PROXY\"\n  type: \"select\"\n  proxies:\n    - \"A\"\n"
	ruleProviders := "{}"
	rules := "- \"MATCH,DIRECT\"\n"

	f.Add(tmpl, proxies, groups, ruleProviders, rules)
	f.Add(tmplCRLF, proxies, groups, ruleProviders, rules)

	f.Fuzz(func(t *testing.T, templateText, proxiesBlock, groupsBlock, ruleProvidersBlock, rulesBlock string) {
		_, _ = InjectAnchors(templateText, render.Blocks{
			Proxies:       proxiesBlock,
			Groups:        groupsBlock,
			RuleProviders: ruleProvidersBlock,
			Rules:         rulesBlock,
		}, AnchorOptions{
			Target:      render.TargetClash,
			TemplateURL: "https://example.com/template.yaml",
		})
	})
}

func FuzzInjectAnchors_Quanx(f *testing.F) {
	tmpl := "[server_local]\n#@PROXIES@#\n[policy]\n#@GROUPS@#\n[filter_remote]\n#@RULESETS@#\n[filter_local]\n#@RULES@#\n"

	proxies := "shadowsocks = example.com:8388, method=aes-128-gcm, password=pass, tag=A\n"
	groups := "static=PROXY, A, direct\n"
	rulesets := "https://example.com/ruleset.list, tag=PROXY, force-policy=PROXY, enabled=true\n"
	rules := "FINAL,direct\n"

	f.Add(tmpl, proxies, groups, rulesets, rules)

	f.Fuzz(func(t *testing.T, templateText, proxiesBlock, groupsBlock, rulesetsBlock, rulesBlock string) {
		_, _ = InjectAnchors(templateText, render.Blocks{
			Proxies:  proxiesBlock,
			Groups:   groupsBlock,
			Rulesets: rulesetsBlock,
			Rules:    rulesBlock,
		}, AnchorOptions{
			Target:      render.TargetQuanx,
			TemplateURL: "https://example.com/template.conf",
		})
	})
}

func FuzzEnsureSurgeManagedConfig(f *testing.F) {
	url := "http://example.com/sub?mode=config&target=surge&sub=https%3A%2F%2Fexample.com%2Fss.txt&profile=https%3A%2F%2Fexample.com%2Fprofile.yaml"
	f.Add("", url)
	f.Add("#!MANAGED-CONFIG http://old.example/sub interval=86400\n[Proxy]\n#@PROXIES@#\n", url)
	f.Add("\n\n[Proxy]\n#@PROXIES@#\n", url)

	f.Fuzz(func(t *testing.T, text, currentURL string) {
		if currentURL == "" {
			currentURL = url
		}
		_, _ = EnsureSurgeManagedConfig(text, currentURL, "https://example.com/template.conf")
	})
}
