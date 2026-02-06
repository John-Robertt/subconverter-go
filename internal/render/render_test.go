package render

import (
	"errors"
	"strings"
	"testing"

	"github.com/John-Robertt/subconverter-go/internal/compiler"
	"github.com/John-Robertt/subconverter-go/internal/model"
)

func TestRender_Clash_PasswordQuotedAndPlugin(t *testing.T) {
	res := &compiler.Result{
		Proxies: []model.Proxy{
			{
				Type:       "ss",
				Name:       "n1",
				Server:     "example.com",
				Port:       8388,
				Cipher:     "aes-128-gcm",
				Password:   "123",
				PluginName: "simple-obfs",
				PluginOpts: []model.KV{{Key: "obfs", Value: "tls"}, {Key: "obfs-host", Value: "example.com"}},
			},
		},
		Groups: []model.Group{
			{Name: "PROXY", Type: "select", Members: []string{"n1", "DIRECT"}},
		},
		Rules: []model.Rule{
			{Type: "MATCH", Action: "PROXY"},
		},
	}

	blocks, err := Render(TargetClash, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(blocks.Proxies, `password: "123"`) {
		t.Fatalf("password should be quoted, got:\n%s", blocks.Proxies)
	}
	if !strings.Contains(blocks.Proxies, "plugin: obfs") {
		t.Fatalf("plugin missing, got:\n%s", blocks.Proxies)
	}
	if !strings.Contains(blocks.Proxies, "plugin-opts:") || !strings.Contains(blocks.Proxies, "mode:") {
		t.Fatalf("plugin-opts missing, got:\n%s", blocks.Proxies)
	}
}

func TestRender_Clash_UnsupportedPlugin(t *testing.T) {
	res := &compiler.Result{
		Proxies: []model.Proxy{
			{Type: "ss", Name: "n1", Server: "example.com", Port: 8388, Cipher: "aes-128-gcm", Password: "pass", PluginName: "v2ray-plugin"},
		},
		Groups: []model.Group{{Name: "PROXY", Type: "select", Members: []string{"n1"}}},
		Rules:  []model.Rule{{Type: "MATCH", Action: "PROXY"}},
	}

	_, err := Render(TargetClash, res)
	var re *RenderError
	if !errors.As(err, &re) {
		t.Fatalf("expected *RenderError, got %T: %v", err, err)
	}
	if re.AppError.Code != "UNSUPPORTED_PLUGIN" {
		t.Fatalf("code=%q, want=%q", re.AppError.Code, "UNSUPPORTED_PLUGIN")
	}
}

func TestRender_SurgeLike_ProxyCommaQuotedAndReferenced(t *testing.T) {
	res := &compiler.Result{
		Proxies: []model.Proxy{
			{Type: "ss", Name: "a,b", Server: "example.com", Port: 8388, Cipher: "aes-128-gcm", Password: "pass"},
		},
		Groups: []model.Group{
			{Name: "PROXY", Type: "select", Members: []string{"a,b", "DIRECT"}},
		},
		Rules: []model.Rule{
			{Type: "MATCH", Action: "PROXY"},
		},
	}

	blocks, err := Render(TargetSurge, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(blocks.Proxies, `"a,b" = ss, example.com, 8388`) {
		t.Fatalf("proxy name should be quoted, got:\n%s", blocks.Proxies)
	}
	if !strings.Contains(blocks.Groups, `PROXY = select, "a,b", DIRECT`) {
		t.Fatalf("group member should reference quoted name, got:\n%s", blocks.Groups)
	}
}

func TestRender_SurgeLike_GroupNameInvalid(t *testing.T) {
	res := &compiler.Result{
		Proxies: []model.Proxy{
			{Type: "ss", Name: "n1", Server: "example.com", Port: 8388, Cipher: "aes-128-gcm", Password: "pass"},
		},
		Groups: []model.Group{
			{Name: "A,B", Type: "select", Members: []string{"n1"}},
		},
		Rules: []model.Rule{{Type: "MATCH", Action: "A,B"}},
	}
	_, err := Render(TargetSurge, res)
	var re *RenderError
	if !errors.As(err, &re) {
		t.Fatalf("expected *RenderError, got %T: %v", err, err)
	}
	if re.AppError.Code != "PROFILE_VALIDATE_ERROR" {
		t.Fatalf("code=%q, want=%q", re.AppError.Code, "PROFILE_VALIDATE_ERROR")
	}
}
