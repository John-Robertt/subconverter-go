package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/John-Robertt/subconverter-go/internal/compiler"
	"github.com/John-Robertt/subconverter-go/internal/fetch"
	"github.com/John-Robertt/subconverter-go/internal/model"
	"github.com/John-Robertt/subconverter-go/internal/profile"
	"github.com/John-Robertt/subconverter-go/internal/render"
	"github.com/John-Robertt/subconverter-go/internal/rules"
	"github.com/John-Robertt/subconverter-go/internal/sub/ss"
	"github.com/John-Robertt/subconverter-go/internal/template"
)

// APIError is used by the HTTP layer for request validation and a few
// HTTP-specific errors.
type APIError struct {
	Status   int
	AppError model.AppError
	Cause    error
}

func (e *APIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.AppError.Code, e.AppError.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.AppError.Code, e.AppError.Message, e.Cause)
}

func (e *APIError) Unwrap() error { return e.Cause }

func apiError(status int, app model.AppError, cause error) error {
	return &APIError{Status: status, AppError: app, Cause: cause}
}

func requestError(code, message, hint string) error {
	return apiError(http.StatusBadRequest, model.AppError{
		Code:    code,
		Message: message,
		Stage:   "validate_request",
		Hint:    hint,
	}, nil)
}

func writeErrorFromErr(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}

	var ae *APIError
	if errors.As(err, &ae) {
		WriteError(w, ae.Status, ae.AppError)
		return
	}

	var fe *fetch.FetchError
	if errors.As(err, &fe) {
		WriteError(w, fe.Status, fe.AppError)
		return
	}

	// Parse/compile/render/template errors are user content errors => 422.
	var se *ss.ParseError
	if errors.As(err, &se) {
		WriteError(w, http.StatusUnprocessableEntity, se.AppError)
		return
	}

	var pe *profile.ParseError
	if errors.As(err, &pe) {
		WriteError(w, http.StatusUnprocessableEntity, pe.AppError)
		return
	}

	var rpe *rules.ParseError
	if errors.As(err, &rpe) {
		WriteError(w, http.StatusUnprocessableEntity, rpe.AppError)
		return
	}

	var ce *compiler.CompileError
	if errors.As(err, &ce) {
		WriteError(w, http.StatusUnprocessableEntity, ce.AppError)
		return
	}

	var re *render.RenderError
	if errors.As(err, &re) {
		WriteError(w, http.StatusUnprocessableEntity, re.AppError)
		return
	}

	var te *template.TemplateError
	if errors.As(err, &te) {
		WriteError(w, http.StatusUnprocessableEntity, te.AppError)
		return
	}

	// Fallback: internal bug.
	WriteError(w, http.StatusInternalServerError, model.AppError{
		Code:    "INTERNAL_ERROR",
		Message: "服务端内部错误",
		Stage:   "internal",
		Hint:    err.Error(),
	})
}
