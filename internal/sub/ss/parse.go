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
	// 1) if it contains substring "ss://", treat as raw list
	// 2) else treat as base64 list and decode.
	if strings.Contains(s, "ss://") {
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
		if !strings.HasPrefix(line, "ss://") {
			return nil, newParseError(sourceURL, i+1, truncateSnippet(orig, 200), "SUB_UNSUPPORTED_SCHEME", "仅支持 ss:// 协议", "expected: ss://...", nil)
		}

		p, err := parseSSURI(sourceURL, i+1, line)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, newParseError(sourceURL, 0, "", "SUB_PARSE_ERROR", "订阅中没有任何可用节点", "", nil)
	}
	return out, nil
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
			// because it makes strict validation ambiguous.
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
