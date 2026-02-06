package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/John-Robertt/subconverter-go/internal/model"
)

func WriteText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func WriteError(w http.ResponseWriter, status int, e model.AppError) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: e})
}
