package template

import (
	"errors"
	"strings"
	"testing"
)

func TestEnsureSurgeManagedConfig_InsertDefault(t *testing.T) {
	in := "" +
		"[General]\n" +
		"loglevel = notify\n"

	out, err := EnsureSurgeManagedConfig(in, "http://example.com/sub?x=1", "https://example.com/surge.conf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(out, "#!MANAGED-CONFIG http://example.com/sub?x=1 interval=86400\n") {
		t.Fatalf("managed-config line not inserted, got:\n%s", out)
	}
	if !strings.Contains(out, "[General]\n") {
		t.Fatalf("original content missing, got:\n%s", out)
	}
}

func TestEnsureSurgeManagedConfig_KeepCRLF(t *testing.T) {
	in := "" +
		"[General]\r\n" +
		"loglevel = notify\r\n"

	out, err := EnsureSurgeManagedConfig(in, "http://example.com/sub?x=1", "https://example.com/surge.conf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "\r\n") {
		t.Fatalf("expected CRLF output, got:\n%q", out)
	}
	if hasBareLF(out) {
		t.Fatalf("output should not contain bare LF, got:\n%q", out)
	}
}

func TestEnsureSurgeManagedConfig_RewriteURL_PreserveParams(t *testing.T) {
	in := "" +
		"#!MANAGED-CONFIG http://old/sub?y=2 interval=123 foo=bar\n" +
		"[General]\n"

	out, err := EnsureSurgeManagedConfig(in, "http://new/sub?x=1", "https://example.com/surge.conf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(out, "#!MANAGED-CONFIG http://new/sub?x=1 interval=123 foo=bar\n") {
		t.Fatalf("managed-config URL not rewritten, got:\n%s", out)
	}
}

func TestEnsureSurgeManagedConfig_MultipleLines_Error(t *testing.T) {
	in := "" +
		"#!MANAGED-CONFIG http://old/sub interval=1 foo=bar\n" +
		"[General]\n" +
		"#!MANAGED-CONFIG http://old2/sub interval=1 foo=bar\n"

	_, err := EnsureSurgeManagedConfig(in, "http://new/sub", "https://example.com/surge.conf")
	var te *TemplateError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TemplateError, got %T: %v", err, err)
	}
	if te.AppError.Code != "TEMPLATE_SECTION_ERROR" {
		t.Fatalf("code=%q, want=%q", te.AppError.Code, "TEMPLATE_SECTION_ERROR")
	}
}

func TestEnsureSurgeManagedConfig_NotFirstNonEmpty_Error(t *testing.T) {
	in := "" +
		"[General]\n" +
		"#!MANAGED-CONFIG http://old/sub interval=1 foo=bar\n"

	_, err := EnsureSurgeManagedConfig(in, "http://new/sub", "https://example.com/surge.conf")
	var te *TemplateError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TemplateError, got %T: %v", err, err)
	}
	if te.AppError.Code != "TEMPLATE_SECTION_ERROR" {
		t.Fatalf("code=%q, want=%q", te.AppError.Code, "TEMPLATE_SECTION_ERROR")
	}
}

func TestEnsureSurgeManagedConfig_MissingURL_Error(t *testing.T) {
	in := "" +
		"#!MANAGED-CONFIG\n" +
		"[General]\n"

	_, err := EnsureSurgeManagedConfig(in, "http://new/sub", "https://example.com/surge.conf")
	var te *TemplateError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TemplateError, got %T: %v", err, err)
	}
	if te.AppError.Code != "TEMPLATE_SECTION_ERROR" {
		t.Fatalf("code=%q, want=%q", te.AppError.Code, "TEMPLATE_SECTION_ERROR")
	}
}
