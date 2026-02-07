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
		metricsIncAppError(ae.AppError.Stage, ae.AppError.Code)
		WriteError(w, ae.Status, ae.AppError)
		return
	}

	var fe *fetch.FetchError
	if errors.As(err, &fe) {
		metricsIncAppError(fe.AppError.Stage, fe.AppError.Code)
		WriteError(w, fe.Status, fe.AppError)
		return
	}

	// Parse/compile/render/template errors are user content errors => 422.
	var se *ss.ParseError
	if errors.As(err, &se) {
		metricsIncAppError(se.AppError.Stage, se.AppError.Code)
		WriteError(w, http.StatusUnprocessableEntity, se.AppError)
		return
	}

	var pe *profile.ParseError
	if errors.As(err, &pe) {
		metricsIncAppError(pe.AppError.Stage, pe.AppError.Code)
		WriteError(w, http.StatusUnprocessableEntity, pe.AppError)
		return
	}

	var ce *compiler.CompileError
	if errors.As(err, &ce) {
		metricsIncAppError(ce.AppError.Stage, ce.AppError.Code)
		WriteError(w, http.StatusUnprocessableEntity, ce.AppError)
		return
	}

	var re *render.RenderError
	if errors.As(err, &re) {
		metricsIncAppError(re.AppError.Stage, re.AppError.Code)
		WriteError(w, http.StatusUnprocessableEntity, re.AppError)
		return
	}

	var te *template.TemplateError
	if errors.As(err, &te) {
		metricsIncAppError(te.AppError.Stage, te.AppError.Code)
		WriteError(w, http.StatusUnprocessableEntity, te.AppError)
		return
	}

	// Fallback: internal bug.
	app := model.AppError{
		Code:    "INTERNAL_ERROR",
		Message: "服务端内部错误",
		Stage:   "internal",
		Hint:    err.Error(),
	}
	metricsIncAppError(app.Stage, app.Code)
	WriteError(w, http.StatusInternalServerError, app)
}
