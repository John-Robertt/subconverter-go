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
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /metrics", handleMetrics)
	mux.HandleFunc("GET /sub", h.handleSub)
	mux.HandleFunc("POST /api/convert", h.handleConvert)
	return mux
}
