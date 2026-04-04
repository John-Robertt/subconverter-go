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
				ID:         "p1",
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
			{Name: "PROXY", Type: "select", Members: []model.MemberRef{proxyRef("p1"), builtinRef("DIRECT")}},
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
			{ID: "p1", Type: "ss", Name: "n1", Server: "example.com", Port: 8388, Cipher: "aes-128-gcm", Password: "pass", PluginName: "v2ray-plugin"},
		},
		Groups: []model.Group{{Name: "PROXY", Type: "select", Members: []model.MemberRef{proxyRef("p1")}}},
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
			{ID: "p1", Type: "ss", Name: "a,b", Server: "example.com", Port: 8388, Cipher: "aes-128-gcm", Password: "pass"},
		},
		Groups: []model.Group{
			{Name: "PROXY", Type: "select", Members: []model.MemberRef{proxyRef("p1"), builtinRef("DIRECT")}},
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
			{ID: "p1", Type: "ss", Name: "n1", Server: "example.com", Port: 8388, Cipher: "aes-128-gcm", Password: "pass"},
		},
		Groups: []model.Group{
			{Name: "A,B", Type: "select", Members: []model.MemberRef{proxyRef("p1")}},
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

func TestRender_Quanx_TagCommaQuotedAndRuleTypeMapping(t *testing.T) {
	res := &compiler.Result{
		Proxies: []model.Proxy{
			{ID: "p1", Type: "ss", Name: "a,b", Server: "example.com", Port: 8388, Cipher: "aes-128-gcm", Password: "pass"},
		},
		Groups: []model.Group{
			{Name: "PROXY", Type: "select", Members: []model.MemberRef{proxyRef("p1"), builtinRef("DIRECT"), builtinRef("REJECT")}},
		},
		Rules: []model.Rule{
			{Type: "IP-CIDR6", Value: "2001:db8::/32", Action: "PROXY", NoResolve: true},
			{Type: "MATCH", Action: "DIRECT"},
		},
	}

	blocks, err := Render(TargetQuanx, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(blocks.Proxies, `tag="a,b"`) {
		t.Fatalf("tag should be quoted, got:\n%s", blocks.Proxies)
	}
	if !strings.Contains(blocks.Groups, `static=PROXY, "a,b", direct, reject`) {
		t.Fatalf("policy group members should be mapped and quoted, got:\n%s", blocks.Groups)
	}
	if !strings.Contains(blocks.Rules, `IP6-CIDR,2001:db8::/32,PROXY,no-resolve`) {
		t.Fatalf("IP-CIDR6 should map to IP6-CIDR, got:\n%s", blocks.Rules)
	}
	if !strings.Contains(blocks.Rules, `FINAL,direct`) {
		t.Fatalf("MATCH should map to FINAL and DIRECT->direct, got:\n%s", blocks.Rules)
	}
}

func TestRender_Quanx_IPv6ServerBracketed(t *testing.T) {
	res := &compiler.Result{
		Proxies: []model.Proxy{
			{ID: "p1", Type: "ss", Name: "v6", Server: "2001:db8::1", Port: 8388, Cipher: "aes-128-gcm", Password: "pass"},
		},
		Groups: []model.Group{
			{Name: "PROXY", Type: "select", Members: []model.MemberRef{proxyRef("p1")}},
		},
		Rules: []model.Rule{
			{Type: "MATCH", Action: "PROXY"},
		},
	}

	blocks, err := Render(TargetQuanx, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(blocks.Proxies, "shadowsocks = [2001:db8::1]:8388") {
		t.Fatalf("IPv6 server should be bracketed, got:\n%s", blocks.Proxies)
	}
}

func TestRender_RejectsMissingProxyID(t *testing.T) {
	res := &compiler.Result{
		Proxies: []model.Proxy{
			{Type: "ss", Name: "n1", Server: "example.com", Port: 8388, Cipher: "aes-128-gcm", Password: "pass"},
		},
		Groups: []model.Group{
			{Name: "PROXY", Type: "select", Members: []model.MemberRef{builtinRef("DIRECT")}},
		},
		Rules: []model.Rule{{Type: "MATCH", Action: "PROXY"}},
	}

	_, err := Render(TargetClash, res)
	var re *RenderError
	if !errors.As(err, &re) {
		t.Fatalf("expected *RenderError, got %T: %v", err, err)
	}
	if re.AppError.Code != "INVALID_ARGUMENT" {
		t.Fatalf("code=%q, want=%q", re.AppError.Code, "INVALID_ARGUMENT")
	}
	if !strings.Contains(re.AppError.Message, "proxy ID") {
		t.Fatalf("message=%q, want mention proxy ID", re.AppError.Message)
	}
}

func TestRender_RejectsDuplicateProxyID(t *testing.T) {
	res := &compiler.Result{
		Proxies: []model.Proxy{
			{ID: "p1", Type: "ss", Name: "n1", Server: "example.com", Port: 8388, Cipher: "aes-128-gcm", Password: "pass"},
			{ID: "p1", Type: "ss", Name: "n2", Server: "example.net", Port: 8389, Cipher: "aes-128-gcm", Password: "pass"},
		},
		Groups: []model.Group{
			{Name: "PROXY", Type: "select", Members: []model.MemberRef{proxyRef("p1")}},
		},
		Rules: []model.Rule{{Type: "MATCH", Action: "PROXY"}},
	}

	_, err := Render(TargetClash, res)
	var re *RenderError
	if !errors.As(err, &re) {
		t.Fatalf("expected *RenderError, got %T: %v", err, err)
	}
	if re.AppError.Code != "INVALID_ARGUMENT" {
		t.Fatalf("code=%q, want=%q", re.AppError.Code, "INVALID_ARGUMENT")
	}
	if !strings.Contains(re.AppError.Message, "必须唯一") {
		t.Fatalf("message=%q, want duplicate ID error", re.AppError.Message)
	}
}

func TestRender_Clash_DerivedHTTPProxyUsesDialerProxy(t *testing.T) {
	res := &compiler.Result{
		Proxies: []model.Proxy{
			{ID: "sub1", Type: "ss", Name: "HK", Server: "hk.example.com", Port: 8388, Cipher: "aes-128-gcm", Password: "pass"},
			{ID: "d1", Type: "http", Name: "CORP-HTTP via HK", Server: "proxy.example.com", Port: 8080, Username: "user", Password: "pass", ViaProxyID: "sub1"},
		},
		Groups: []model.Group{{Name: "CHAIN-CORP-HTTP", Type: "select", Members: []model.MemberRef{proxyRef("d1")}}},
		Rules:  []model.Rule{{Type: "MATCH", Action: "CHAIN-CORP-HTTP"}},
	}

	blocks, err := Render(TargetClash, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(blocks.Proxies, `type: http`) || !strings.Contains(blocks.Proxies, `dialer-proxy: "HK"`) {
		t.Fatalf("derived clash proxy missing chain fields, got:\n%s", blocks.Proxies)
	}
	if !strings.Contains(blocks.Proxies, `username: "user"`) {
		t.Fatalf("derived clash proxy missing auth fields, got:\n%s", blocks.Proxies)
	}
}

func TestRender_Surge_DerivedHTTPProxyUsesUnderlyingProxy(t *testing.T) {
	res := &compiler.Result{
		Proxies: []model.Proxy{
			{ID: "sub1", Type: "ss", Name: "HK", Server: "hk.example.com", Port: 8388, Cipher: "aes-128-gcm", Password: "pass"},
			{ID: "d1", Type: "http", Name: "CORP-HTTP via HK", Server: "proxy.example.com", Port: 8080, Username: "user", Password: "pass", ViaProxyID: "sub1"},
		},
		Groups: []model.Group{{Name: "CHAIN-CORP-HTTP", Type: "select", Members: []model.MemberRef{proxyRef("d1")}}},
		Rules:  []model.Rule{{Type: "MATCH", Action: "CHAIN-CORP-HTTP"}},
	}

	blocks, err := Render(TargetSurge, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(blocks.Proxies, `CORP-HTTP via HK = http, proxy.example.com, 8080, user, pass, underlying-proxy=HK`) {
		t.Fatalf("derived surge proxy missing chain fields, got:\n%s", blocks.Proxies)
	}
}

func TestRender_Surge_RejectsCommaInProxyCredentials(t *testing.T) {
	res := &compiler.Result{
		Proxies: []model.Proxy{
			{ID: "d1", Type: "http", Name: "CORP-HTTP", Server: "proxy.example.com", Port: 8080, Username: "user", Password: "pa,ss"},
		},
		Groups: []model.Group{{Name: "PROXY", Type: "select", Members: []model.MemberRef{proxyRef("d1")}}},
		Rules:  []model.Rule{{Type: "MATCH", Action: "PROXY"}},
	}

	_, err := Render(TargetSurge, res)
	var re *RenderError
	if !errors.As(err, &re) {
		t.Fatalf("expected *RenderError, got %T: %v", err, err)
	}
	if re.AppError.Code != "SUB_PARSE_ERROR" {
		t.Fatalf("code=%q, want=%q", re.AppError.Code, "SUB_PARSE_ERROR")
	}
}

func TestRender_Shadowrocket_RejectsProxyChain(t *testing.T) {
	res := &compiler.Result{
		Proxies: []model.Proxy{
			{ID: "sub1", Type: "ss", Name: "HK", Server: "hk.example.com", Port: 8388, Cipher: "aes-128-gcm", Password: "pass"},
			{ID: "d1", Type: "http", Name: "CORP-HTTP via HK", Server: "proxy.example.com", Port: 8080, ViaProxyID: "sub1"},
		},
		Groups: []model.Group{{Name: "CHAIN-CORP-HTTP", Type: "select", Members: []model.MemberRef{proxyRef("d1")}}},
		Rules:  []model.Rule{{Type: "MATCH", Action: "CHAIN-CORP-HTTP"}},
	}

	_, err := Render(TargetShadowrocket, res)
	var re *RenderError
	if !errors.As(err, &re) {
		t.Fatalf("expected *RenderError, got %T: %v", err, err)
	}
	if re.AppError.Code != "UNSUPPORTED_TARGET_FEATURE" {
		t.Fatalf("code=%q, want=%q", re.AppError.Code, "UNSUPPORTED_TARGET_FEATURE")
	}
}

func TestRender_Quanx_RejectsProxyChain(t *testing.T) {
	res := &compiler.Result{
		Proxies: []model.Proxy{
			{ID: "sub1", Type: "ss", Name: "HK", Server: "hk.example.com", Port: 8388, Cipher: "aes-128-gcm", Password: "pass"},
			{ID: "d1", Type: "http", Name: "CORP-HTTP via HK", Server: "proxy.example.com", Port: 8080, ViaProxyID: "sub1"},
		},
		Groups: []model.Group{{Name: "CHAIN-CORP-HTTP", Type: "select", Members: []model.MemberRef{proxyRef("d1")}}},
		Rules:  []model.Rule{{Type: "MATCH", Action: "CHAIN-CORP-HTTP"}},
	}

	_, err := Render(TargetQuanx, res)
	var re *RenderError
	if !errors.As(err, &re) {
		t.Fatalf("expected *RenderError, got %T: %v", err, err)
	}
	if re.AppError.Code != "UNSUPPORTED_TARGET_FEATURE" {
		t.Fatalf("code=%q, want=%q", re.AppError.Code, "UNSUPPORTED_TARGET_FEATURE")
	}
}

func proxyRef(id string) model.MemberRef {
	return model.MemberRef{Kind: model.MemberRefProxy, Value: id}
}

func builtinRef(name string) model.MemberRef {
	return model.MemberRef{Kind: model.MemberRefBuiltin, Value: name}
}
