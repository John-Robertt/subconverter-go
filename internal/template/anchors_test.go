package template

import (
	"errors"
	"strings"
	"testing"

	"github.com/John-Robertt/subconverter-go/internal/render"
)

func TestInjectAnchors_OK_IndentAndCRLFPreserved(t *testing.T) {
	templateText := "" +
		"[General]\r\n" +
		"loglevel = notify\r\n" +
		"\r\n" +
		"[Proxy]\r\n" +
		"  #@PROXIES@#\r\n" +
		"\r\n" +
		"[Proxy Group]\r\n" +
		"\t#@GROUPS@#\r\n" +
		"\r\n" +
		"[Rule]\r\n" +
		"#@RULES@#\r\n"

	blocks := render.Blocks{
		Proxies: "A = ss, a.com, 1, encrypt-method=aes-128-gcm, password=p\nB = ss, b.com, 2, encrypt-method=aes-128-gcm, password=p",
		Groups:  "PROXY = select, A, DIRECT",
		Rules:   "FINAL,DIRECT",
	}

	out, err := InjectAnchors(templateText, blocks, AnchorOptions{
		Target:      render.TargetSurge,
		TemplateURL: "https://example.com/surge.conf",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "\r\n") {
		t.Fatalf("expected CRLF output, got:\n%q", out)
	}
	if hasBareLF(out) {
		t.Fatalf("output should not contain bare LF, got:\n%q", out)
	}
	// Indentation inherited from anchor lines.
	if !strings.Contains(out, "  A = ss") || !strings.Contains(out, "  B = ss") {
		t.Fatalf("proxy block indent not preserved, got:\n%s", out)
	}
	if !strings.Contains(out, "\tPROXY = select") {
		t.Fatalf("group block indent not preserved, got:\n%s", out)
	}
}

func TestInjectAnchors_MissingAnchor(t *testing.T) {
	templateText := "" +
		"[Proxy]\n" +
		"#@PROXIES@#\n" +
		"[Rule]\n" +
		"#@RULES@#\n"

	_, err := InjectAnchors(templateText, render.Blocks{}, AnchorOptions{
		Target:      render.TargetSurge,
		TemplateURL: "https://example.com/surge.conf",
	})
	var te *TemplateError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TemplateError, got %T: %v", err, err)
	}
	if te.AppError.Code != "TEMPLATE_ANCHOR_MISSING" {
		t.Fatalf("code=%q, want=%q", te.AppError.Code, "TEMPLATE_ANCHOR_MISSING")
	}
}

func TestInjectAnchors_DupAnchor(t *testing.T) {
	templateText := "" +
		"[Proxy]\n" +
		"#@PROXIES@#\n" +
		"[Proxy Group]\n" +
		"#@GROUPS@#\n" +
		"#@GROUPS@#\n" +
		"[Rule]\n" +
		"#@RULES@#\n"

	_, err := InjectAnchors(templateText, render.Blocks{}, AnchorOptions{
		Target:      render.TargetSurge,
		TemplateURL: "https://example.com/surge.conf",
	})
	var te *TemplateError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TemplateError, got %T: %v", err, err)
	}
	if te.AppError.Code != "TEMPLATE_ANCHOR_DUP" {
		t.Fatalf("code=%q, want=%q", te.AppError.Code, "TEMPLATE_ANCHOR_DUP")
	}
}

func TestInjectAnchors_AnchorNotStandalone(t *testing.T) {
	templateText := "" +
		"[Proxy]\n" +
		"#@PROXIES@# extra\n" +
		"[Proxy Group]\n" +
		"#@GROUPS@#\n" +
		"[Rule]\n" +
		"#@RULES@#\n"

	_, err := InjectAnchors(templateText, render.Blocks{}, AnchorOptions{
		Target:      render.TargetSurge,
		TemplateURL: "https://example.com/surge.conf",
	})
	var te *TemplateError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TemplateError, got %T: %v", err, err)
	}
	if te.AppError.Code != "TEMPLATE_SECTION_ERROR" {
		t.Fatalf("code=%q, want=%q", te.AppError.Code, "TEMPLATE_SECTION_ERROR")
	}
	if !strings.Contains(te.AppError.Message, "独占一行") {
		t.Fatalf("message=%q, want contains %q", te.AppError.Message, "独占一行")
	}
}

func TestInjectAnchors_SurgeSectionError(t *testing.T) {
	templateText := "" +
		"[General]\n" +
		"#@PROXIES@#\n" +
		"[Proxy Group]\n" +
		"#@GROUPS@#\n" +
		"[Rule]\n" +
		"#@RULES@#\n"

	_, err := InjectAnchors(templateText, render.Blocks{}, AnchorOptions{
		Target:      render.TargetSurge,
		TemplateURL: "https://example.com/surge.conf",
	})
	var te *TemplateError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TemplateError, got %T: %v", err, err)
	}
	if te.AppError.Code != "TEMPLATE_SECTION_ERROR" {
		t.Fatalf("code=%q, want=%q", te.AppError.Code, "TEMPLATE_SECTION_ERROR")
	}
}

func TestInjectAnchors_ClashAnchorIndentZero(t *testing.T) {
	templateText := "" +
		"proxies:\n" +
		"#@PROXIES@#\n" +
		"proxy-groups:\n" +
		"  #@GROUPS@#\n" +
		"rule-providers:\n" +
		"  #@RULE_PROVIDERS@#\n" +
		"rules:\n" +
		"  #@RULES@#\n"

	_, err := InjectAnchors(templateText, render.Blocks{}, AnchorOptions{
		Target:      render.TargetClash,
		TemplateURL: "https://example.com/clash.yaml",
	})
	var te *TemplateError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TemplateError, got %T: %v", err, err)
	}
	if te.AppError.Code != "TEMPLATE_SECTION_ERROR" {
		t.Fatalf("code=%q, want=%q", te.AppError.Code, "TEMPLATE_SECTION_ERROR")
	}
	if !strings.Contains(te.AppError.Message, "缩进") {
		t.Fatalf("message=%q, want contains %q", te.AppError.Message, "缩进")
	}
}

func TestInjectAnchors_QuanxSectionError(t *testing.T) {
	templateText := "" +
		"[policy]\n" +
		"#@GROUPS@#\n" +
		"[server_local]\n" +
		"#@PROXIES@#\n" +
		"[general]\n" +
		"#@RULES@#\n"

	_, err := InjectAnchors(templateText, render.Blocks{}, AnchorOptions{
		Target:      render.TargetQuanx,
		TemplateURL: "https://example.com/quanx.conf",
	})
	var te *TemplateError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TemplateError, got %T: %v", err, err)
	}
	if te.AppError.Code != "TEMPLATE_SECTION_ERROR" {
		t.Fatalf("code=%q, want=%q", te.AppError.Code, "TEMPLATE_SECTION_ERROR")
	}
}

func hasBareLF(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '\n' {
			continue
		}
		if i == 0 || s[i-1] != '\r' {
			return true
		}
	}
	return false
}
