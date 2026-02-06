package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/John-Robertt/subconverter-go/internal/compiler"
	"github.com/John-Robertt/subconverter-go/internal/fetch"
	"github.com/John-Robertt/subconverter-go/internal/model"
	"github.com/John-Robertt/subconverter-go/internal/profile"
	"github.com/John-Robertt/subconverter-go/internal/render"
	"github.com/John-Robertt/subconverter-go/internal/sub/ss"
	"github.com/John-Robertt/subconverter-go/internal/template"
)

type convertRequest struct {
	Mode      string
	Target    render.Target
	Subs      []string
	Profile   string
	Encode    string // only for mode=list: "base64" | "raw"
	isFromGET bool   // used for error hinting; not part of API
}

type convertRequestJSON struct {
	Mode    string   `json:"mode"`
	Target  string   `json:"target"`
	Subs    []string `json:"subs"`
	Profile string   `json:"profile"`
	Encode  string   `json:"encode"`
}

func runConvert(ctx context.Context, r *http.Request, req convertRequest) (string, error) {
	// Keep a hard upper bound so handlers don't hang forever if upstream misbehaves.
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	subs, err := fetchAndParseSubs(ctx, req.Subs)
	if err != nil {
		return "", err
	}

	switch req.Mode {
	case "list":
		proxies, err := compiler.NormalizeSubscriptionProxies(subs)
		if err != nil {
			return "", err
		}

		rawList, err := renderSSListRaw(proxies)
		if err != nil {
			return "", err
		}

		encode := req.Encode
		if encode == "" {
			encode = "base64"
		}
		switch encode {
		case "raw":
			return rawList, nil
		case "base64":
			return base64.StdEncoding.EncodeToString([]byte(rawList)), nil
		default:
			return "", requestError("INVALID_ARGUMENT", "不支持的 encode（仅支持 base64/raw）", encode)
		}
	case "config":
		prof, err := fetchAndParseProfile(ctx, req.Profile, string(req.Target))
		if err != nil {
			return "", err
		}

		res, err := compiler.Compile(ctx, subs, prof)
		if err != nil {
			return "", err
		}

		blocks, err := render.Render(req.Target, res)
		if err != nil {
			return "", err
		}

		templateURL := prof.Template[string(req.Target)]
		templateText, err := fetch.FetchText(ctx, fetch.KindTemplate, templateURL)
		if err != nil {
			return "", err
		}

		out, err := template.InjectAnchors(templateText, blocks, template.AnchorOptions{
			Target:      req.Target,
			TemplateURL: templateURL,
		})
		if err != nil {
			return "", err
		}

		if req.Target == render.TargetSurge {
			currentURL, err := buildSurgeManagedConfigURL(r, req, prof.PublicBaseURL)
			if err != nil {
				return "", err
			}
			out, err = template.EnsureSurgeManagedConfig(out, currentURL, templateURL)
			if err != nil {
				return "", err
			}
		}

		return out, nil
	default:
		return "", requestError("INVALID_ARGUMENT", "不支持的 mode（仅支持 config/list）", req.Mode)
	}
}

func fetchAndParseSubs(ctx context.Context, subURLs []string) ([]model.Proxy, error) {
	out := make([]model.Proxy, 0)
	for _, raw := range subURLs {
		u := strings.TrimSpace(raw)
		if u == "" {
			return nil, requestError("INVALID_ARGUMENT", "sub 不能为空", "")
		}
		text, err := fetch.FetchText(ctx, fetch.KindSubscription, u)
		if err != nil {
			return nil, err
		}
		proxies, err := ss.ParseSubscriptionText(u, text)
		if err != nil {
			return nil, err
		}
		out = append(out, proxies...)
	}
	if len(out) == 0 {
		return nil, &compiler.CompileError{
			AppError: model.AppError{
				Code:    "SUB_PARSE_ERROR",
				Message: "订阅中没有任何可用节点",
				Stage:   "compile",
			},
		}
	}
	return out, nil
}

func fetchAndParseProfile(ctx context.Context, profileURL string, requiredTarget string) (*profile.Spec, error) {
	profileURL = strings.TrimSpace(profileURL)
	if profileURL == "" {
		return nil, requestError("INVALID_ARGUMENT", "profile 不能为空", "")
	}
	text, err := fetch.FetchText(ctx, fetch.KindProfile, profileURL)
	if err != nil {
		return nil, err
	}
	return profile.ParseProfileYAML(profileURL, text, requiredTarget)
}

func renderSSListRaw(proxies []model.Proxy) (string, error) {
	if len(proxies) == 0 {
		return "", errors.New("empty proxies list")
	}
	lines := make([]string, 0, len(proxies))
	for _, p := range proxies {
		line, err := canonicalSSURI(p)
		if err != nil {
			return "", err
		}
		lines = append(lines, line)
	}
	// v1 spec: raw output must end with a newline.
	return strings.Join(lines, "\n") + "\n", nil
}

func canonicalSSURI(p model.Proxy) (string, error) {
	if p.Type != "ss" {
		return "", fmt.Errorf("unsupported proxy type: %s", p.Type)
	}
	userInfo := strings.ToLower(p.Cipher) + ":" + p.Password
	userB64 := base64.RawURLEncoding.EncodeToString([]byte(userInfo))

	host := p.Server
	// IPv6 host must be wrapped in [] in URI.
	if strings.Contains(host, ":") && !(strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]")) {
		host = "[" + host + "]"
	}

	var b strings.Builder
	b.WriteString("ss://")
	b.WriteString(userB64)
	b.WriteString("@")
	b.WriteString(host)
	b.WriteByte(':')
	b.WriteString(strconv.Itoa(p.Port))

	if strings.TrimSpace(p.PluginName) != "" {
		var pb strings.Builder
		pb.WriteString(strings.TrimSpace(p.PluginName))
		for _, kv := range p.PluginOpts {
			pb.WriteByte(';')
			pb.WriteString(strings.TrimSpace(kv.Key))
			pb.WriteByte('=')
			pb.WriteString(strings.TrimSpace(kv.Value))
		}
		b.WriteString("/?plugin=")
		b.WriteString(pctEncode(pb.String()))
	}

	if p.Name != "" {
		b.WriteByte('#')
		b.WriteString(pctEncode(p.Name))
	}
	return b.String(), nil
}

func pctEncode(s string) string {
	// RFC 3986 percent-encoding for query/fragment. Go's QueryEscape uses '+' for
	// spaces, which we rewrite to %20 for stability and to avoid ambiguity.
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func buildSurgeManagedConfigURL(r *http.Request, req convertRequest, publicBaseURL string) (string, error) {
	if req.Mode != "config" || req.Target != render.TargetSurge {
		return "", requestError("INVALID_ARGUMENT", "仅 mode=config&target=surge 需要 managed-config URL", "")
	}
	if len(req.Subs) == 0 || strings.TrimSpace(req.Profile) == "" {
		return "", requestError("INVALID_ARGUMENT", "生成 managed-config URL 需要 sub/profile", "")
	}

	base := strings.TrimSpace(publicBaseURL)
	if base == "" {
		base = deriveRequestBaseURL(r) + "/sub"
	}

	u, err := url.Parse(base)
	if err != nil || u == nil || !u.IsAbs() {
		return "", apiError(http.StatusUnprocessableEntity, model.AppError{
			Code:    "PROFILE_VALIDATE_ERROR",
			Message: "public_base_url 不合法，无法生成 managed-config URL",
			Stage:   "compile",
			Snippet: base,
		}, errors.Join(errors.New("invalid public_base_url"), err))
	}

	// Deterministic query serialization (SPEC_DETERMINISM.md):
	// 1) mode=config
	// 2) target=surge
	// 3) sub=... in input order
	// 4) profile=...
	u.RawQuery = serializeQuery([]kv{
		{k: "mode", v: "config"},
		{k: "target", v: "surge"},
	}, req.Subs, req.Profile)
	u.Fragment = ""
	return u.String(), nil
}

type kv struct {
	k string
	v string
}

func serializeQuery(prefix []kv, subs []string, profileURL string) string {
	parts := make([]kv, 0, len(prefix)+len(subs)+1)
	parts = append(parts, prefix...)
	for _, s := range subs {
		parts = append(parts, kv{k: "sub", v: s})
	}
	parts = append(parts, kv{k: "profile", v: profileURL})

	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(p.k)
		b.WriteByte('=')
		b.WriteString(pctEncode(p.v))
	}
	return b.String()
}

func deriveRequestBaseURL(r *http.Request) string {
	if r == nil {
		return "http://127.0.0.1:25500"
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "127.0.0.1:25500"
	}
	return scheme + "://" + host
}

func parseConvertGET(r *http.Request) (convertRequest, error) {
	q := r.URL.Query()
	for key := range q {
		switch key {
		case "mode", "target", "sub", "profile", "encode":
		default:
			return convertRequest{}, requestError("INVALID_ARGUMENT", fmt.Sprintf("不支持的 query 参数：%s", key), "")
		}
	}

	mode, err := singleQuery(q, "mode", true)
	if err != nil {
		return convertRequest{}, err
	}
	mode = strings.TrimSpace(mode)
	if mode != "config" && mode != "list" {
		return convertRequest{}, requestError("INVALID_ARGUMENT", "不支持的 mode（仅支持 config/list）", mode)
	}

	subs := q["sub"]
	if len(subs) == 0 {
		return convertRequest{}, requestError("INVALID_ARGUMENT", "缺少 sub 参数", "expected: sub=<url>")
	}
	subs2 := make([]string, 0, len(subs))
	for _, s := range subs {
		s = strings.TrimSpace(s)
		if s == "" {
			return convertRequest{}, requestError("INVALID_ARGUMENT", "sub 不能为空", "")
		}
		subs2 = append(subs2, s)
	}

	if mode == "list" {
		if _, ok := q["target"]; ok {
			return convertRequest{}, requestError("INVALID_ARGUMENT", "mode=list 不支持 target", "")
		}
		if _, ok := q["profile"]; ok {
			return convertRequest{}, requestError("INVALID_ARGUMENT", "mode=list 不支持 profile", "")
		}
		encode, err := singleQuery(q, "encode", false)
		if err != nil {
			return convertRequest{}, err
		}
		if encode == "" {
			encode = "base64"
		}
		if encode != "base64" && encode != "raw" {
			return convertRequest{}, requestError("INVALID_ARGUMENT", "不支持的 encode（仅支持 base64/raw）", encode)
		}
		return convertRequest{Mode: "list", Subs: subs2, Encode: encode, isFromGET: true}, nil
	}

	// mode=config
	if _, ok := q["encode"]; ok {
		return convertRequest{}, requestError("INVALID_ARGUMENT", "mode=config 不支持 encode", "")
	}
	targetStr, err := singleQuery(q, "target", true)
	if err != nil {
		return convertRequest{}, err
	}
	target, err := parseTarget(targetStr)
	if err != nil {
		return convertRequest{}, err
	}
	profileURL, err := singleQuery(q, "profile", true)
	if err != nil {
		return convertRequest{}, err
	}
	profileURL = strings.TrimSpace(profileURL)
	if profileURL == "" {
		return convertRequest{}, requestError("INVALID_ARGUMENT", "profile 不能为空", "")
	}
	return convertRequest{
		Mode:      "config",
		Target:    target,
		Subs:      subs2,
		Profile:   profileURL,
		isFromGET: true,
	}, nil
}

func parseConvertPOST(r *http.Request) (convertRequest, error) {
	var body convertRequestJSON
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		return convertRequest{}, requestError("INVALID_ARGUMENT", "JSON body 解析失败", err.Error())
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return convertRequest{}, requestError("INVALID_ARGUMENT", "JSON body 不允许多段", "")
	} else if !errors.Is(err, io.EOF) {
		return convertRequest{}, requestError("INVALID_ARGUMENT", "JSON body 解析失败", err.Error())
	}

	mode := strings.TrimSpace(body.Mode)
	if mode != "config" && mode != "list" {
		return convertRequest{}, requestError("INVALID_ARGUMENT", "不支持的 mode（仅支持 config/list）", mode)
	}
	if len(body.Subs) == 0 {
		return convertRequest{}, requestError("INVALID_ARGUMENT", "subs 不能为空", "")
	}
	subs := make([]string, 0, len(body.Subs))
	for _, s := range body.Subs {
		s = strings.TrimSpace(s)
		if s == "" {
			return convertRequest{}, requestError("INVALID_ARGUMENT", "subs 不能为空", "")
		}
		subs = append(subs, s)
	}

	if mode == "list" {
		if strings.TrimSpace(body.Target) != "" {
			return convertRequest{}, requestError("INVALID_ARGUMENT", "mode=list 不支持 target", "")
		}
		if strings.TrimSpace(body.Profile) != "" {
			return convertRequest{}, requestError("INVALID_ARGUMENT", "mode=list 不支持 profile", "")
		}
		encode := strings.TrimSpace(body.Encode)
		if encode == "" {
			encode = "base64"
		}
		if encode != "base64" && encode != "raw" {
			return convertRequest{}, requestError("INVALID_ARGUMENT", "不支持的 encode（仅支持 base64/raw）", encode)
		}
		return convertRequest{Mode: "list", Subs: subs, Encode: encode}, nil
	}

	// mode=config
	if strings.TrimSpace(body.Encode) != "" {
		return convertRequest{}, requestError("INVALID_ARGUMENT", "mode=config 不支持 encode", "")
	}

	target, err := parseTarget(body.Target)
	if err != nil {
		return convertRequest{}, err
	}
	profileURL := strings.TrimSpace(body.Profile)
	if profileURL == "" {
		return convertRequest{}, requestError("INVALID_ARGUMENT", "profile 不能为空", "")
	}
	return convertRequest{Mode: "config", Target: target, Subs: subs, Profile: profileURL}, nil
}

func parseTarget(s string) (render.Target, error) {
	switch strings.TrimSpace(s) {
	case string(render.TargetClash):
		return render.TargetClash, nil
	case string(render.TargetShadowrocket):
		return render.TargetShadowrocket, nil
	case string(render.TargetSurge):
		return render.TargetSurge, nil
	default:
		return "", requestError("INVALID_ARGUMENT", "不支持的 target（仅支持 clash/shadowrocket/surge）", s)
	}
}

func singleQuery(q url.Values, key string, required bool) (string, error) {
	values, ok := q[key]
	if !ok || len(values) == 0 {
		if required {
			return "", requestError("INVALID_ARGUMENT", fmt.Sprintf("缺少 %s 参数", key), "")
		}
		return "", nil
	}
	if len(values) != 1 {
		return "", requestError("INVALID_ARGUMENT", fmt.Sprintf("%s 参数只能出现一次", key), "")
	}
	return values[0], nil
}
