package httpapi

import "net/http"

func NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleIndex)
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /sub", handleSub)
	mux.HandleFunc("POST /api/convert", handleConvert)
	return mux
}
