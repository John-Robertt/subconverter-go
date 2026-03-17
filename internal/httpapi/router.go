package httpapi

import "net/http"

func NewMux() *http.ServeMux {
	return NewMuxWithOptions(Options{})
}

func NewMuxWithOptions(opt Options) *http.ServeMux {
	opt = opt.withDefaults()
	h := convertHandler{opt: opt}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleIndex)
	mux.HandleFunc("GET /healthz", h.handleHealthz)
	mux.HandleFunc("GET /logs/errors.zip", h.handleErrorLogsZip)
	mux.HandleFunc("GET /metrics", handleMetrics)
	mux.HandleFunc("GET /sub", h.handleSub)
	mux.HandleFunc("POST /api/convert", h.handleConvert)
	return mux
}
