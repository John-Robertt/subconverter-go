package template

import (
	"fmt"
	"strings"

	"github.com/John-Robertt/subconverter-go/internal/model"
)

const managedConfigPrefix = "#!MANAGED-CONFIG"

// EnsureSurgeManagedConfig ensures the first non-empty line is:
//
//	#!MANAGED-CONFIG <currentConvertURL> interval=86400
//
// Rules are defined in docs/spec/SPEC_TEMPLATE_ANCHORS.md.
func EnsureSurgeManagedConfig(text string, currentConvertURL string, templateURL string) (string, error) {
	if strings.TrimSpace(currentConvertURL) == "" {
		return "", &TemplateError{
			AppError: model.AppError{
				Code:    "INVALID_ARGUMENT",
				Message: "currentConvertURL 不能为空",
				Stage:   "validate_template",
				URL:     templateURL,
			},
		}
	}
	if text == "" {
		return "", &TemplateError{
			AppError: model.AppError{
				Code:    "INVALID_ARGUMENT",
				Message: "template 不能为空",
				Stage:   "validate_template",
				URL:     templateURL,
			},
		}
	}

	newline := detectNewline(text)
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	endsWithNewline := strings.HasSuffix(normalized, "\n")
	lines := strings.Split(normalized, "\n")

	managedLine := -1
	for i, line := range lines {
		if isManagedConfigLine(line) {
			if managedLine != -1 {
				return "", managedConfigAmbiguous(templateURL, "模板包含多条 #!MANAGED-CONFIG")
			}
			managedLine = i
		}
	}

	if managedLine != -1 {
		firstNonEmpty := firstNonEmptyLine(lines)
		if firstNonEmpty == -1 {
			return "", &TemplateError{
				AppError: model.AppError{
					Code:    "TEMPLATE_SECTION_ERROR",
					Message: "template 不能为空",
					Stage:   "validate_template",
					URL:     templateURL,
				},
			}
		}
		if managedLine != firstNonEmpty {
			return "", managedConfigAmbiguous(templateURL, "#!MANAGED-CONFIG 必须是第一个非空行")
		}

		rewritten, err := rewriteManagedConfigURL(lines[managedLine], currentConvertURL, templateURL)
		if err != nil {
			return "", err
		}
		lines[managedLine] = rewritten
	} else {
		// Insert with v1 default params.
		defaultLine := fmt.Sprintf("%s %s interval=86400", managedConfigPrefix, currentConvertURL)
		lines = append([]string{defaultLine}, lines...)
	}

	out := strings.Join(lines, "\n")
	if !endsWithNewline {
		out = strings.TrimSuffix(out, "\n")
	}
	if newline == "\r\n" {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	return out, nil
}

func isManagedConfigLine(line string) bool {
	trimLeft := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(trimLeft, managedConfigPrefix)
}

func firstNonEmptyLine(lines []string) int {
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			return i
		}
	}
	return -1
}

func rewriteManagedConfigURL(line string, newURL string, templateURL string) (string, error) {
	leadLen := len(line) - len(strings.TrimLeft(line, " \t"))
	lead := line[:leadLen]
	rest := line[leadLen:]
	if !strings.HasPrefix(rest, managedConfigPrefix) {
		return "", &TemplateError{
			AppError: model.AppError{
				Code:    "TEMPLATE_SECTION_ERROR",
				Message: "无法解析 #!MANAGED-CONFIG 行",
				Stage:   "validate_template",
				URL:     templateURL,
				Snippet: line,
			},
		}
	}
	after := rest[len(managedConfigPrefix):]

	// Parse "<ws><url><rest>" and only rewrite the URL token.
	i := 0
	for i < len(after) && (after[i] == ' ' || after[i] == '\t') {
		i++
	}
	if i == len(after) {
		return "", managedConfigAmbiguous(templateURL, "#!MANAGED-CONFIG 缺少 URL")
	}
	urlStart := i

	j := urlStart
	for j < len(after) && after[j] != ' ' && after[j] != '\t' {
		j++
	}
	urlEnd := j

	rewritten := after[:urlStart] + newURL + after[urlEnd:]
	return lead + managedConfigPrefix + rewritten, nil
}

func managedConfigAmbiguous(templateURL, msg string) error {
	return &TemplateError{
		AppError: model.AppError{
			Code:    "TEMPLATE_SECTION_ERROR",
			Message: msg,
			Stage:   "validate_template",
			URL:     templateURL,
			Hint:    "Surge managed config 必须唯一且位于第一个非空行",
		},
	}
}
