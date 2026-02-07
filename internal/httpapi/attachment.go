package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/John-Robertt/subconverter-go/internal/render"
)

func setAttachmentHeaders(w http.ResponseWriter, req convertRequest) error {
	filename, err := outputFileName(req)
	if err != nil {
		return err
	}
	if filename == "" {
		return nil
	}
	// Add both filename and filename* for better UTF-8 compatibility.
	w.Header().Set("Content-Disposition", contentDispositionAttachment(filename))
	return nil
}

func outputFileName(req convertRequest) (string, error) {
	base := strings.TrimSpace(req.FileName)
	if base == "" {
		base = defaultBaseName(req)
	}
	if base == "" {
		return "", nil
	}
	if strings.ContainsAny(base, "\r\n\x00") {
		return "", requestError("INVALID_ARGUMENT", "fileName 含有非法控制字符", "")
	}
	if strings.Contains(base, "/") || strings.Contains(base, "\\") {
		return "", requestError("INVALID_ARGUMENT", "fileName 不允许包含路径分隔符", "")
	}
	if len(base) > 200 {
		return "", requestError("INVALID_ARGUMENT", "fileName 过长", "max=200 bytes")
	}

	name := base
	if !hasExt(name) {
		if ext := defaultExt(req); ext != "" {
			name += ext
		}
	}
	return name, nil
}

func defaultBaseName(req convertRequest) string {
	switch req.Mode {
	case "list":
		// SS subscription list output.
		return "ss"
	case "config":
		if req.Target != "" {
			return string(req.Target)
		}
	}
	return ""
}

func hasExt(name string) bool {
	i := strings.LastIndexByte(name, '.')
	return i > 0 && i < len(name)-1
}

func defaultExt(req convertRequest) string {
	switch req.Mode {
	case "list":
		return ".txt"
	case "config":
		switch req.Target {
		case render.TargetClash:
			return ".yaml"
		case render.TargetSurge, render.TargetShadowrocket, render.TargetQuanx:
			return ".conf"
		default:
			return ""
		}
	default:
		return ""
	}
}

func contentDispositionAttachment(filename string) string {
	// RFC 6266 + RFC 5987.
	escaped := strings.ReplaceAll(filename, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")

	// pctEncode follows our deterministic encoding (space => %20).
	return fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", escaped, pctEncode(filename))
}
