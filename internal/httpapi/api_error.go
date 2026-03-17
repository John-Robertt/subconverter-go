package httpapi

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/John-Robertt/subconverter-go/internal/compiler"
	"github.com/John-Robertt/subconverter-go/internal/errlog"
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
	return apiError(http.StatusBadRequest, newAppError(code, message, "validate_request", withHint(hint)), nil)
}

type classifiedError struct {
	status int
	app    model.AppError
	cause  error
}

func writeErrorFromErr(w http.ResponseWriter, r *http.Request, err error, collector *errlog.Collector, store *errlog.Store) {
	if err == nil {
		return
	}

	ce := classifyError(err)
	metricsIncAppError(ce.app.Stage, ce.app.Code)

	if collector != nil && store != nil {
		record := collector.BuildFailure(time.Now(), ce.status, ce.app, ce.cause)
		if writeErr := store.WriteFailure(record); writeErr != nil {
			log.Printf("write error snapshot failed: %v", writeErr)
		}
	}

	WriteError(w, ce.status, ce.app)
}

func classifyError(err error) classifiedError {
	var ae *APIError
	if errors.As(err, &ae) {
		return classifiedError{status: ae.Status, app: ae.AppError, cause: ae.Cause}
	}

	var fe *fetch.FetchError
	if errors.As(err, &fe) {
		return classifiedError{status: fe.Status, app: fe.AppError, cause: fe.Cause}
	}

	// Parse/compile/render/template errors are user content errors => 422.
	var se *ss.ParseError
	if errors.As(err, &se) {
		return classifiedError{status: http.StatusUnprocessableEntity, app: se.AppError, cause: se.Cause}
	}

	var pe *profile.ParseError
	if errors.As(err, &pe) {
		return classifiedError{status: http.StatusUnprocessableEntity, app: pe.AppError, cause: pe.Cause}
	}

	var ce *compiler.CompileError
	if errors.As(err, &ce) {
		return classifiedError{status: http.StatusUnprocessableEntity, app: ce.AppError, cause: ce.Cause}
	}

	var re *render.RenderError
	if errors.As(err, &re) {
		return classifiedError{status: http.StatusUnprocessableEntity, app: re.AppError, cause: re.Cause}
	}

	var te *template.TemplateError
	if errors.As(err, &te) {
		return classifiedError{status: http.StatusUnprocessableEntity, app: te.AppError, cause: te.Cause}
	}

	// Fallback: internal bug.
	app := newAppError("INTERNAL_ERROR", "服务端内部错误", "internal", withHint(err.Error()))
	return classifiedError{status: http.StatusInternalServerError, app: app, cause: err}
}

type appErrorOption func(*model.AppError)

func withHint(hint string) appErrorOption {
	return func(app *model.AppError) {
		app.Hint = hint
	}
}

func withSnippet(snippet string) appErrorOption {
	return func(app *model.AppError) {
		app.Snippet = snippet
	}
}

func newAppError(code, message, stage string, opts ...appErrorOption) model.AppError {
	app := model.AppError{
		Code:    code,
		Message: message,
		Stage:   stage,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&app)
		}
	}
	return app
}
