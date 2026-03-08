package ss

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/John-Robertt/subconverter-go/internal/model"
)

type ParseError struct {
	AppError model.AppError
	Cause    error
}

func (e *ParseError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.AppError.Code, e.AppError.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.AppError.Code, e.AppError.Message, e.Cause)
}

func (e *ParseError) Unwrap() error { return e.Cause }

func ParseSubscriptionText(sourceURL string, content string) ([]model.Proxy, error) {
	s := stripUTF8BOM(content)
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, newParseError(sourceURL, 0, "", "SUB_PARSE_ERROR", "订阅内容为空", "", nil)
	}

	// Auto-detect rule from docs/spec/SPEC_SUBSCRIPTION_SS.md:
	// 1) if it looks like a raw list (ss:// or other supported raw formats), parse directly
	// 2) else treat as base64 list and decode.
	if looksLikeRawList(s) {
		return parseRawList(sourceURL, s)
	}

	decoded, err := decodeSubscriptionBase64(s)
	if err != nil {
		return nil, newParseError(sourceURL, 0, truncateSnippet(s, 200), "SUB_BASE64_DECODE_ERROR", "订阅 base64 解码失败", "", err)
	}
	decoded = stripUTF8BOM(decoded)
	decoded = strings.TrimSpace(decoded)
	if decoded == "" {
		return nil, newParseError(sourceURL, 0, "", "SUB_PARSE_ERROR", "订阅内容为空", "", nil)
	}
	return parseRawList(sourceURL, decoded)
}

func looksLikeRawList(s string) bool {
	// Fast path: the canonical raw list format.
	if strings.Contains(s, "ss://") {
		return true
	}

	// Slow path: look at the first meaningful line to avoid accidental false positives.
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Shadowrocket-style: <name>=ss, <server>, <port>, ...
		// Some providers emit an extra blank after '=' ("= ss,").
		if looksLikeShadowrocketSSLine(line) {
			return true
		}

		// Surge-style: shadowsocks = <server>:<port>, method=..., password=..., tag=...
		left, _, hasEq := strings.Cut(line, "=")
		if hasEq && strings.EqualFold(strings.TrimSpace(left), "shadowsocks") {
			return true
		}

		// Deterministic: only inspect the first meaningful line.
		break
	}
	return false
}

func looksLikeShadowrocketSSLine(line string) bool {
	_, rest, ok := strings.Cut(line, "=")
	if !ok {
		return false
	}
	rest = strings.TrimSpace(rest)
	return strings.HasPrefix(rest, "ss,")
}

func normalizeKVValue(v string) (string, error) {
	v = strings.TrimSpace(v)
	if len(v) < 2 {
		return v, nil
	}
	quote := v[0]
	if quote != '"' && quote != 0x27 && quote != '`' {
		return v, nil
	}
	if v[len(v)-1] != quote {
		return "", errors.New("unterminated quoted value")
	}
	return strconv.Unquote(v)
}

func parseRawList(sourceURL, raw string) ([]model.Proxy, error) {
	// Use \n split and trim trailing \r to be CRLF-compatible.
	lines := strings.Split(raw, "\n")
	out := make([]model.Proxy, 0, len(lines))
	for i, line := range lines {
		orig := line
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}

		var (
			p   model.Proxy
			err error
			ok  bool
		)
		switch {
		case strings.HasPrefix(line, "ss://"):
			p, err = parseSSURI(sourceURL, i+1, line)
			if err != nil {
				return nil, err
			}
		default:
			p, ok, err = parseShadowrocketSSLine(sourceURL, i+1, line)
			if err != nil {
				return nil, err
			}
			if !ok {
				p, ok, err = parseSurgeShadowsocksLine(sourceURL, i+1, line)
				if err != nil {
					return nil, err
				}
			}
			if !ok {
				return nil, newParseError(sourceURL, i+1, truncateSnippet(orig, 200), "SUB_UNSUPPORTED_SCHEME", "不支持的订阅行格式", "expected: ss://... | <name>=ss,... | shadowsocks = ...", nil)
			}
		}

		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, newParseError(sourceURL, 0, "", "SUB_PARSE_ERROR", "订阅中没有任何可用节点", "", nil)
	}
	return out, nil
}

func parseShadowrocketSSLine(sourceURL string, lineNo int, line string) (model.Proxy, bool, error) {
	// Shadowrocket subscription line:
	//   <name>=ss, <server>, <port>, encrypt-method=<cipher>, password=<password>, [obfs=<mode>], [obfs-host=<host>], ...
	namePart, rest, ok := strings.Cut(line, "=")
	if !ok {
		return model.Proxy{}, false, nil
	}

	name := strings.TrimSpace(namePart)
	if strings.ContainsAny(name, "\r\n\x00") {
		return model.Proxy{}, true, newParseError(sourceURL, lineNo, truncateSnippet(line, 200), "SUB_PARSE_ERROR", "节点名称包含非法控制字符", "forbidden: \\r \\n \\0", nil)
	}

	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(rest, "ss,") {
		return model.Proxy{}, false, nil
	}
	rest = strings.TrimSpace(strings.TrimPrefix(rest, "ss,"))
	if rest == "" {
		return model.Proxy{}, true, newParseError(sourceURL, lineNo, truncateSnippet(line, 200), "SUB_PARSE_ERROR", "ss 行缺少 server/port", "example: name=ss, example.com, 8388, encrypt-method=aes-128-gcm, password=pass", nil)
	}

	parts := strings.Split(rest, ",")
	trimmed := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		trimmed = append(trimmed, p)
	}
	if len(trimmed) < 2 {
		return model.Proxy{}, true, newParseError(sourceURL, lineNo, truncateSnippet(line, 200), "SUB_PARSE_ERROR", "ss 行缺少 server/port", "example: name=ss, example.com, 8388, encrypt-method=aes-128-gcm, password=pass", nil)
	}

	server := strings.TrimSpace(trimmed[0])
	portStr := strings.TrimSpace(trimmed[1])
	portInt, err := strconv.Atoi(portStr)
	if err != nil || portInt < 1 || portInt > 65535 {
		return model.Proxy{}, true, newParseError(sourceURL, lineNo, truncateSnippet(line, 200), "SUB_PARSE_ERROR", "服务器端口不合法", "expected: 1..65535", err)
	}

	var (
		method   string
		password string
		obfs     string
		obfsHost string
	)
	for _, seg := range trimmed[2:] {
		k, v, hasEq := strings.Cut(seg, "=")
		if !hasEq {
			// Keep this strict: a bare token is ambiguous.
			return model.Proxy{}, true, newParseError(sourceURL, lineNo, truncateSnippet(line, 200), "SUB_PARSE_ERROR", "ss 行参数必须是 key=value 形式", "example: encrypt-method=aes-128-gcm", nil)
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v, err = normalizeKVValue(v)
		if err != nil {
			return model.Proxy{}, true, newParseError(sourceURL, lineNo, truncateSnippet(line, 200), "SUB_PARSE_ERROR", "ss 行参数值引号不合法", "example: password=pass or password=\"pass\"", err)
		}
		switch k {
		case "encrypt-method", "method":
			method = v
		case "password":
			password = v
		case "obfs":
			obfs = v
		case "obfs-host":
			obfsHost = v
		default:
			// Ignore unsupported knobs (tfo/udp-relay/...) to keep conversion useful.
		}
	}
	if strings.TrimSpace(method) == "" || strings.TrimSpace(password) == "" {
		return model.Proxy{}, true, newParseError(sourceURL, lineNo, truncateSnippet(line, 200), "SUB_PARSE_ERROR", "ss 行缺少必需字段 encrypt-method/method 或 password", "required: encrypt-method=<cipher>, password=<password>", nil)
	}

	var pluginName string
	var pluginOpts []model.KV
	if strings.TrimSpace(obfs) != "" {
		pluginName = "simple-obfs"
		pluginOpts = append(pluginOpts, model.KV{Key: "obfs", Value: obfs})
		if strings.TrimSpace(obfsHost) != "" {
			pluginOpts = append(pluginOpts, model.KV{Key: "obfs-host", Value: obfsHost})
		}
	}

	return model.Proxy{
		Type:       "ss",
		Name:       name,
		Server:     server,
		Port:       portInt,
		Cipher:     method,
		Password:   password,
		PluginName: pluginName,
		PluginOpts: pluginOpts,
	}, true, nil
}

func parseSurgeShadowsocksLine(sourceURL string, lineNo int, line string) (model.Proxy, bool, error) {
	// Surge proxy line:
	//   shadowsocks = <server>:<port>, method=<cipher>, password=<password>, tag=<name>, ...
	left, right, ok := strings.Cut(line, "=")
	if !ok {
		return model.Proxy{}, false, nil
	}
	if !strings.EqualFold(strings.TrimSpace(left), "shadowsocks") {
		return model.Proxy{}, false, nil
	}

	right = strings.TrimSpace(right)
	if right == "" {
		return model.Proxy{}, true, newParseError(sourceURL, lineNo, truncateSnippet(line, 200), "SUB_PARSE_ERROR", "shadowsocks 行为空", "example: shadowsocks = example.com:8388, method=aes-128-gcm, password=pass, tag=HK", nil)
	}

	parts := strings.Split(right, ",")
	if len(parts) == 0 {
		return model.Proxy{}, true, newParseError(sourceURL, lineNo, truncateSnippet(line, 200), "SUB_PARSE_ERROR", "shadowsocks 行缺少 server:port", "example: shadowsocks = example.com:8388, method=aes-128-gcm, password=pass, tag=HK", nil)
	}
	hostPort := strings.TrimSpace(parts[0])
	server, portInt, err := parseHostPort(hostPort)
	if err != nil {
		return model.Proxy{}, true, newParseError(sourceURL, lineNo, truncateSnippet(line, 200), "SUB_PARSE_ERROR", "服务器地址或端口不合法", "expected: host:port", err)
	}

	var (
		method   string
		password string
		name     string
		obfs     string
		obfsHost string
	)
	for _, seg := range parts[1:] {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		k, v, hasEq := strings.Cut(seg, "=")
		if !hasEq {
			return model.Proxy{}, true, newParseError(sourceURL, lineNo, truncateSnippet(line, 200), "SUB_PARSE_ERROR", "shadowsocks 行参数必须是 key=value 形式", "example: method=aes-128-gcm", nil)
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v, err = normalizeKVValue(v)
		if err != nil {
			return model.Proxy{}, true, newParseError(sourceURL, lineNo, truncateSnippet(line, 200), "SUB_PARSE_ERROR", "shadowsocks 行参数值引号不合法", "example: password=pass or password=\"pass\"", err)
		}
		switch k {
		case "encrypt-method", "method":
			method = v
		case "password":
			password = v
		case "tag":
			name = v
		case "obfs":
			obfs = v
		case "obfs-host":
			obfsHost = v
		default:
			// Ignore unsupported knobs (fast-open/udp-relay/...) to keep conversion useful.
		}
	}
	if strings.ContainsAny(name, "\r\n\x00") {
		return model.Proxy{}, true, newParseError(sourceURL, lineNo, truncateSnippet(line, 200), "SUB_PARSE_ERROR", "节点名称包含非法控制字符", "forbidden: \\r \\n \\0", nil)
	}
	if strings.TrimSpace(method) == "" || strings.TrimSpace(password) == "" {
		return model.Proxy{}, true, newParseError(sourceURL, lineNo, truncateSnippet(line, 200), "SUB_PARSE_ERROR", "shadowsocks 行缺少必需字段 method 或 password", "required: method=<cipher>, password=<password>", nil)
	}

	var pluginName string
	var pluginOpts []model.KV
	if strings.TrimSpace(obfs) != "" {
		pluginName = "simple-obfs"
		pluginOpts = append(pluginOpts, model.KV{Key: "obfs", Value: obfs})
		if strings.TrimSpace(obfsHost) != "" {
			pluginOpts = append(pluginOpts, model.KV{Key: "obfs-host", Value: obfsHost})
		}
	}

	return model.Proxy{
		Type:       "ss",
		Name:       strings.TrimSpace(name),
		Server:     server,
		Port:       portInt,
		Cipher:     method,
		Password:   password,
		PluginName: pluginName,
		PluginOpts: pluginOpts,
	}, true, nil
}

func parseSSURI(sourceURL string, lineNo int, s string) (model.Proxy, error) {
	// Split fragment first: #name
	withoutFrag, frag, hasFrag := strings.Cut(s, "#")
	name := ""
	if hasFrag {
		decoded, err := url.PathUnescape(frag)
		if err != nil {
			return model.Proxy{}, newParseError(sourceURL, lineNo, truncateSnippet(s, 200), "SUB_PARSE_ERROR", "节点名称 URL 解码失败", "", err)
		}
		name = strings.TrimSpace(decoded)
		if strings.ContainsAny(name, "\r\n\x00") {
			return model.Proxy{}, newParseError(sourceURL, lineNo, truncateSnippet(s, 200), "SUB_PARSE_ERROR", "节点名称包含非法控制字符", "forbidden: \\r \\n \\0", nil)
		}
	}

	withoutQuery, query, hasQuery := strings.Cut(withoutFrag, "?")
	pluginName, pluginOpts, err := parseQueryPlugin(sourceURL, lineNo, query, hasQuery, s)
	if err != nil {
		return model.Proxy{}, err
	}

	rest := strings.TrimPrefix(withoutQuery, "ss://")
	if rest == "" {
		return model.Proxy{}, newParseError(sourceURL, lineNo, truncateSnippet(s, 200), "SUB_PARSE_ERROR", "ss:// 后缺少内容", "", nil)
	}

	// Form A: <b64(method:password)>@<host>:<port>
	if strings.Contains(rest, "@") {
		userB64, hostPart, ok := strings.Cut(rest, "@")
		if !ok || userB64 == "" || hostPart == "" {
			return model.Proxy{}, newParseError(sourceURL, lineNo, truncateSnippet(s, 200), "SUB_PARSE_ERROR", "ss uri 格式不合法", "", nil)
		}

		hostPort := hostPart
		if idx := strings.IndexByte(hostPort, '/'); idx >= 0 {
			// Only allow empty path or a single trailing "/".
			if hostPort[idx:] != "/" {
				return model.Proxy{}, newParseError(sourceURL, lineNo, truncateSnippet(s, 200), "SUB_PARSE_ERROR", "ss uri path 不支持（仅允许空或 /）", "", nil)
			}
			hostPort = hostPort[:idx]
		}

		method, password, err := decodeMethodPassword(userB64)
		if err != nil {
			return model.Proxy{}, newParseError(sourceURL, lineNo, truncateSnippet(s, 200), "SUB_PARSE_ERROR", "ss userinfo base64 解码失败", "", err)
		}

		server, port, err := parseHostPort(hostPort)
		if err != nil {
			return model.Proxy{}, newParseError(sourceURL, lineNo, truncateSnippet(s, 200), "SUB_PARSE_ERROR", "服务器地址或端口不合法", "", err)
		}

		return model.Proxy{
			Type:       "ss",
			Name:       name,
			Server:     server,
			Port:       port,
			Cipher:     method,
			Password:   password,
			PluginName: pluginName,
			PluginOpts: pluginOpts,
		}, nil
	}

	// Form B: ss://<b64(method:password@host:port)>
	decoded, err := decodeB64ToString(rest)
	if err != nil {
		return model.Proxy{}, newParseError(sourceURL, lineNo, truncateSnippet(s, 200), "SUB_PARSE_ERROR", "ss base64 解码失败", "", err)
	}
	if !utf8.ValidString(decoded) {
		return model.Proxy{}, newParseError(sourceURL, lineNo, truncateSnippet(s, 200), "SUB_PARSE_ERROR", "ss base64 解码结果不是合法 UTF-8", "", nil)
	}

	at := strings.LastIndex(decoded, "@")
	if at < 0 {
		return model.Proxy{}, newParseError(sourceURL, lineNo, truncateSnippet(s, 200), "SUB_PARSE_ERROR", "ss base64 解码结果缺少 @ 分隔符", "", nil)
	}
	credPart := decoded[:at]
	hostPortPart := decoded[at+1:]

	colon := strings.IndexByte(credPart, ':')
	if colon <= 0 {
		return model.Proxy{}, newParseError(sourceURL, lineNo, truncateSnippet(s, 200), "SUB_PARSE_ERROR", "ss base64 解码结果缺少 cipher:password", "", nil)
	}
	method := strings.TrimSpace(credPart[:colon])
	password := strings.TrimSpace(credPart[colon+1:])
	if method == "" || password == "" {
		return model.Proxy{}, newParseError(sourceURL, lineNo, truncateSnippet(s, 200), "SUB_PARSE_ERROR", "cipher 或 password 不能为空", "", nil)
	}
	if strings.ContainsAny(method, "\r\n\x00") || strings.ContainsAny(password, "\r\n\x00") {
		return model.Proxy{}, newParseError(sourceURL, lineNo, truncateSnippet(s, 200), "SUB_PARSE_ERROR", "cipher 或 password 包含非法控制字符", "", nil)
	}

	server, port, err := parseHostPort(hostPortPart)
	if err != nil {
		return model.Proxy{}, newParseError(sourceURL, lineNo, truncateSnippet(s, 200), "SUB_PARSE_ERROR", "服务器地址或端口不合法", "", err)
	}

	return model.Proxy{
		Type:       "ss",
		Name:       name,
		Server:     server,
		Port:       port,
		Cipher:     method,
		Password:   password,
		PluginName: pluginName,
		PluginOpts: pluginOpts,
	}, nil
}

func parseQueryPlugin(sourceURL string, lineNo int, query string, hasQuery bool, fullLine string) (string, []model.KV, error) {
	if !hasQuery || query == "" {
		return "", nil, nil
	}

	// net/url.ParseQuery rejects non-URL-encoded semicolons, but SIP002 plugin
	// uses semicolons inside the "plugin" value. So we parse query manually and
	// only support '&' as separator.
	var pluginValue *string

	parts := strings.Split(query, "&")
	for _, part := range parts {
		if part == "" {
			continue
		}
		kRaw, vRaw, hasEq := strings.Cut(part, "=")
		if !hasEq {
			// Unlike net/url.ParseQuery we do not accept key-without-=
			// because it makes validation ambiguous.
			return "", nil, newParseError(sourceURL, lineNo, truncateSnippet(fullLine, 200), "SUB_PARSE_ERROR", "query 参数必须是 key=value 形式", "", nil)
		}
		k, err := url.PathUnescape(kRaw)
		if err != nil {
			return "", nil, newParseError(sourceURL, lineNo, truncateSnippet(fullLine, 200), "SUB_PARSE_ERROR", "query 参数解码失败", "", err)
		}
		v, err := url.PathUnescape(vRaw)
		if err != nil {
			return "", nil, newParseError(sourceURL, lineNo, truncateSnippet(fullLine, 200), "SUB_PARSE_ERROR", "query 参数解码失败", "", err)
		}

		if k != "plugin" {
			return "", nil, newParseError(sourceURL, lineNo, truncateSnippet(fullLine, 200), "SUB_PARSE_ERROR", "出现未知 query 参数（仅支持 plugin）", "only allow: plugin", nil)
		}
		if pluginValue != nil {
			return "", nil, newParseError(sourceURL, lineNo, truncateSnippet(fullLine, 200), "SUB_PARSE_ERROR", "重复的 plugin 参数", "", nil)
		}
		pluginValue = &v
	}

	if pluginValue == nil {
		return "", nil, nil
	}
	if strings.TrimSpace(*pluginValue) == "" {
		return "", nil, newParseError(sourceURL, lineNo, truncateSnippet(fullLine, 200), "SUB_PARSE_ERROR", "plugin 参数不能为空", "", nil)
	}

	segs := strings.Split(*pluginValue, ";")
	pluginName := strings.TrimSpace(segs[0])
	if pluginName == "" {
		return "", nil, newParseError(sourceURL, lineNo, truncateSnippet(fullLine, 200), "SUB_PARSE_ERROR", "plugin 名称不能为空", "", nil)
	}
	opts := make([]model.KV, 0, len(segs)-1)
	for _, seg := range segs[1:] {
		if seg == "" {
			continue
		}
		k, v, ok := strings.Cut(seg, "=")
		if !ok {
			return "", nil, newParseError(sourceURL, lineNo, truncateSnippet(fullLine, 200), "SUB_PARSE_ERROR", "plugin 选项必须是 k=v 形式", "", nil)
		}
		k = strings.TrimSpace(k)
		// Keep v as-is (including spaces) after percent-decoding.
		if k == "" {
			return "", nil, newParseError(sourceURL, lineNo, truncateSnippet(fullLine, 200), "SUB_PARSE_ERROR", "plugin 选项 key 不能为空", "", nil)
		}
		opts = append(opts, model.KV{Key: k, Value: v})
	}
	return pluginName, opts, nil
}

func parseHostPort(s string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		return "", 0, err
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return "", 0, errors.New("empty host")
	}
	portInt, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil {
		return "", 0, err
	}
	if portInt < 1 || portInt > 65535 {
		return "", 0, errors.New("port out of range")
	}
	return host, portInt, nil
}

func decodeMethodPassword(userB64 string) (string, string, error) {
	decoded, err := decodeB64ToString(userB64)
	if err != nil {
		return "", "", err
	}
	if !utf8.ValidString(decoded) {
		return "", "", errors.New("decoded method:password is not valid utf-8")
	}
	colon := strings.IndexByte(decoded, ':')
	if colon <= 0 {
		return "", "", errors.New("missing ':'")
	}
	method := strings.TrimSpace(decoded[:colon])
	password := strings.TrimSpace(decoded[colon+1:])
	if method == "" || password == "" {
		return "", "", errors.New("empty method or password")
	}
	if strings.ContainsAny(method, "\r\n\x00") || strings.ContainsAny(password, "\r\n\x00") {
		return "", "", errors.New("control chars in method/password")
	}
	return method, password, nil
}

func decodeSubscriptionBase64(s string) (string, error) {
	// Remove all whitespace (space/tab/CR/LF) before decoding.
	s2 := removeSpaceTabCRLF(s)
	b, err := decodeB64ToBytes(s2)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) {
		return "", errors.New("decoded subscription is not valid utf-8")
	}
	return string(b), nil
}

func decodeB64ToString(s string) (string, error) {
	b, err := decodeB64ToBytes(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeB64ToBytes(s string) ([]byte, error) {
	// Try standard alphabet (with padding) first, then URL-safe, then raw (no padding).
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	}
	var lastErr error
	for _, enc := range encodings {
		b, err := enc.DecodeString(s)
		if err == nil {
			return b, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func removeSpaceTabCRLF(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\r', '\n':
			continue
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func stripUTF8BOM(s string) string {
	if strings.HasPrefix(s, "\uFEFF") {
		return strings.TrimPrefix(s, "\uFEFF")
	}
	return s
}

func truncateSnippet(s string, max int) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func newParseError(sourceURL string, lineNo int, snippet string, code string, message string, hint string, cause error) error {
	return &ParseError{
		AppError: model.AppError{
			Code:    code,
			Message: message,
			Stage:   "parse_sub",
			URL:     sourceURL,
			Line:    lineNo,
			Snippet: snippet,
			Hint:    hint,
		},
		Cause: cause,
	}
}
