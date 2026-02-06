package template

import (
	"fmt"

	"github.com/John-Robertt/subconverter-go/internal/model"
)

type TemplateError struct {
	AppError model.AppError
	Cause    error
}

func (e *TemplateError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.AppError.Code, e.AppError.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.AppError.Code, e.AppError.Message, e.Cause)
}

func (e *TemplateError) Unwrap() error { return e.Cause }

