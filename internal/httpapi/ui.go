package httpapi

import (
	_ "embed"
	"net/http"
)

//go:embed ui/index.html
var uiIndexHTML []byte

func handleIndex(w http.ResponseWriter, r *http.Request) {
	// Keep UI as a single embedded file so the binary stays self-contained.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(uiIndexHTML)
}
