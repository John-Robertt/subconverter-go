package httpapi

import (
	"net/http"

	"github.com/John-Robertt/subconverter-go/internal/model"
)

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	WriteText(w, http.StatusOK, "ok\n")
}

func handleSub(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusInternalServerError, model.AppError{
		Code:    "NOT_IMPLEMENTED",
		Message: "阶段 0：/sub 尚未实现",
		Stage:   "validate_request",
	})
}

func handleConvert(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusInternalServerError, model.AppError{
		Code:    "NOT_IMPLEMENTED",
		Message: "阶段 0：/api/convert 尚未实现",
		Stage:   "validate_request",
	})
}
