package template

import (
	"fmt"
	"strings"

	"github.com/John-Robertt/subconverter-go/internal/model"
	"github.com/John-Robertt/subconverter-go/internal/render"
)

const (
	AnchorProxies       = "#@PROXIES@#"
	AnchorGroups        = "#@GROUPS@#"
	AnchorRulesets      = "#@RULESETS@#"
	AnchorRuleProviders = "#@RULE_PROVIDERS@#"
	AnchorRules         = "#@RULES@#"
)

type AnchorOptions struct {
	Target      render.Target
	TemplateURL string
}

// InjectAnchors validates anchors and injects blocks into the template.
// It preserves indentation (leading whitespace) and newline style (CRLF/LF).
func InjectAnchors(templateText string, blocks render.Blocks, opt AnchorOptions) (string, error) {
	if templateText == "" {
		return "", &TemplateError{
			AppError: model.AppError{
				Code:    "INVALID_ARGUMENT",
				Message: "template 不能为空",
				Stage:   "validate_template",
				URL:     opt.TemplateURL,
			},
		}
	}

	newline := detectNewline(templateText)
	normalized := strings.ReplaceAll(templateText, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	endsWithNewline := strings.HasSuffix(normalized, "\n")

	pos, err := findAndValidateAnchors(lines, opt.Target, opt.TemplateURL)
	if err != nil {
		return "", err
	}

	lines[pos.proxiesLine] = indentBlock(lines[pos.proxiesLine], blocks.Proxies)
	lines[pos.groupsLine] = indentBlock(lines[pos.groupsLine], blocks.Groups)
	if pos.ruleProvidersLine != -1 {
		lines[pos.ruleProvidersLine] = indentBlock(lines[pos.ruleProvidersLine], blocks.RuleProviders)
	}
	if pos.rulesetsLine != -1 {
		lines[pos.rulesetsLine] = indentBlock(lines[pos.rulesetsLine], blocks.Rulesets)
	}
	lines[pos.rulesLine] = indentBlock(lines[pos.rulesLine], blocks.Rules)

	out := strings.Join(lines, "\n")
	if !endsWithNewline {
		out = strings.TrimSuffix(out, "\n")
	}
	if newline == "\r\n" {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	return out, nil
}

type anchorPos struct {
	proxiesLine       int
	groupsLine        int
	ruleProvidersLine int
	rulesetsLine      int
	rulesLine         int
}

func findAndValidateAnchors(lines []string, target render.Target, templateURL string) (anchorPos, error) {
	pos := anchorPos{proxiesLine: -1, groupsLine: -1, ruleProvidersLine: -1, rulesetsLine: -1, rulesLine: -1}
	countP, countG, countRP, countRS, countR := 0, 0, 0, 0, 0

	section := ""
	for i, line := range lines {
		// Fail fast if an anchor appears but is not standalone.
		if strings.Contains(line, AnchorProxies) && strings.TrimSpace(line) != AnchorProxies {
			return anchorPos{}, anchorNotStandalone(templateURL, line, AnchorProxies)
		}
		if strings.Contains(line, AnchorGroups) && strings.TrimSpace(line) != AnchorGroups {
			return anchorPos{}, anchorNotStandalone(templateURL, line, AnchorGroups)
		}
		if strings.Contains(line, AnchorRulesets) && strings.TrimSpace(line) != AnchorRulesets {
			return anchorPos{}, anchorNotStandalone(templateURL, line, AnchorRulesets)
		}
		if strings.Contains(line, AnchorRuleProviders) && strings.TrimSpace(line) != AnchorRuleProviders {
			return anchorPos{}, anchorNotStandalone(templateURL, line, AnchorRuleProviders)
		}
		if strings.Contains(line, AnchorRules) && strings.TrimSpace(line) != AnchorRules {
			return anchorPos{}, anchorNotStandalone(templateURL, line, AnchorRules)
		}

		trim := strings.TrimSpace(line)
		if sec, ok := parseSectionHeader(trim); ok {
			section = sec
			continue
		}

		switch trim {
		case AnchorProxies:
			countP++
			pos.proxiesLine = i
			if target == render.TargetSurge || target == render.TargetShadowrocket {
				if section != "proxy" {
					return anchorPos{}, sectionError(templateURL, fmt.Sprintf("%s 必须位于 [Proxy] 段内", AnchorProxies))
				}
			}
			if target == render.TargetQuanx {
				if section != "server_local" {
					return anchorPos{}, sectionError(templateURL, fmt.Sprintf("%s 必须位于 [server_local] 段内", AnchorProxies))
				}
			}
		case AnchorGroups:
			countG++
			pos.groupsLine = i
			if target == render.TargetSurge || target == render.TargetShadowrocket {
				if section != "proxy group" {
					return anchorPos{}, sectionError(templateURL, fmt.Sprintf("%s 必须位于 [Proxy Group] 段内", AnchorGroups))
				}
			}
			if target == render.TargetQuanx {
				if section != "policy" {
					return anchorPos{}, sectionError(templateURL, fmt.Sprintf("%s 必须位于 [policy] 段内", AnchorGroups))
				}
			}
		case AnchorRuleProviders:
			countRP++
			pos.ruleProvidersLine = i
			if target != render.TargetClash {
				return anchorPos{}, sectionError(templateURL, fmt.Sprintf("%s 仅支持 Clash 模板（target=clash）", AnchorRuleProviders))
			}
		case AnchorRulesets:
			countRS++
			pos.rulesetsLine = i
			if target != render.TargetQuanx {
				return anchorPos{}, sectionError(templateURL, fmt.Sprintf("%s 仅支持 Quantumult X 模板（target=quanx）", AnchorRulesets))
			}
			if section != "filter_remote" {
				return anchorPos{}, sectionError(templateURL, fmt.Sprintf("%s 必须位于 [filter_remote] 段内", AnchorRulesets))
			}
		case AnchorRules:
			countR++
			pos.rulesLine = i
			if target == render.TargetSurge || target == render.TargetShadowrocket {
				if section != "rule" {
					return anchorPos{}, sectionError(templateURL, fmt.Sprintf("%s 必须位于 [Rule] 段内", AnchorRules))
				}
			}
			if target == render.TargetQuanx {
				if section != "filter_local" {
					return anchorPos{}, sectionError(templateURL, fmt.Sprintf("%s 必须位于 [filter_local] 段内", AnchorRules))
				}
			}
		}
	}

	if countP == 0 {
		return anchorPos{}, anchorMissing(templateURL, AnchorProxies)
	}
	if countG == 0 {
		return anchorPos{}, anchorMissing(templateURL, AnchorGroups)
	}
	if countR == 0 {
		return anchorPos{}, anchorMissing(templateURL, AnchorRules)
	}
	if target == render.TargetClash && countRP == 0 {
		return anchorPos{}, anchorMissing(templateURL, AnchorRuleProviders)
	}
	if countP > 1 {
		return anchorPos{}, anchorDup(templateURL, AnchorProxies)
	}
	if countG > 1 {
		return anchorPos{}, anchorDup(templateURL, AnchorGroups)
	}
	if countRP > 1 {
		return anchorPos{}, anchorDup(templateURL, AnchorRuleProviders)
	}
	if countRS > 1 {
		return anchorPos{}, anchorDup(templateURL, AnchorRulesets)
	}
	if countR > 1 {
		return anchorPos{}, anchorDup(templateURL, AnchorRules)
	}

	// Clash YAML minimal check: anchor indent should not be 0.
	if target == render.TargetClash {
		if leadingWhitespace(lines[pos.proxiesLine]) == "" || leadingWhitespace(lines[pos.groupsLine]) == "" || leadingWhitespace(lines[pos.ruleProvidersLine]) == "" || leadingWhitespace(lines[pos.rulesLine]) == "" {
			return anchorPos{}, sectionError(templateURL, "Clash 模板锚点缩进不能为 0（应位于对应列表下方）")
		}
	}

	if target == render.TargetQuanx {
		if countRS == 0 {
			return anchorPos{}, anchorMissing(templateURL, AnchorRulesets)
		}
	}

	return pos, nil
}

func anchorMissing(templateURL, anchor string) error {
	return &TemplateError{
		AppError: model.AppError{
			Code:    "TEMPLATE_ANCHOR_MISSING",
			Message: fmt.Sprintf("缺少锚点 %s", anchor),
			Stage:   "validate_template",
			URL:     templateURL,
		},
	}
}

func anchorNotStandalone(templateURL, line, anchor string) error {
	return &TemplateError{
		AppError: model.AppError{
			Code:    "TEMPLATE_SECTION_ERROR",
			Message: "锚点必须独占一行",
			Stage:   "validate_template",
			URL:     templateURL,
			Snippet: line,
			Hint:    anchor,
		},
	}
}

func anchorDup(templateURL, anchor string) error {
	return &TemplateError{
		AppError: model.AppError{
			Code:    "TEMPLATE_ANCHOR_DUP",
			Message: fmt.Sprintf("锚点 %s 重复出现", anchor),
			Stage:   "validate_template",
			URL:     templateURL,
		},
	}
}

func sectionError(templateURL, msg string) error {
	return &TemplateError{
		AppError: model.AppError{
			Code:    "TEMPLATE_SECTION_ERROR",
			Message: msg,
			Stage:   "validate_template",
			URL:     templateURL,
		},
	}
}

func indentBlock(anchorLine string, block string) string {
	indent := leadingWhitespace(anchorLine)
	if block == "" {
		return ""
	}
	blockLines := strings.Split(block, "\n")
	for i := range blockLines {
		blockLines[i] = indent + blockLines[i]
	}
	return strings.Join(blockLines, "\n")
}

func parseSectionHeader(trim string) (string, bool) {
	if len(trim) < 3 {
		return "", false
	}
	if trim[0] != '[' || trim[len(trim)-1] != ']' {
		return "", false
	}
	inner := strings.ToLower(strings.TrimSpace(trim[1 : len(trim)-1]))
	return inner, true
}

func leadingWhitespace(line string) string {
	i := 0
	for i < len(line) {
		if line[i] == ' ' || line[i] == '\t' {
			i++
			continue
		}
		break
	}
	return line[:i]
}

func detectNewline(s string) string {
	if strings.Contains(s, "\r\n") {
		return "\r\n"
	}
	return "\n"
}
