package render

import (
	"fmt"

	"github.com/John-Robertt/subconverter-go/internal/compiler"
	"github.com/John-Robertt/subconverter-go/internal/model"
)

type Target string

const (
	TargetClash        Target = "clash"
	TargetSurge        Target = "surge"
	TargetShadowrocket Target = "shadowrocket"
	TargetQuanx        Target = "quanx"
)

type Blocks struct {
	Proxies string
	Groups  string
	Rulesets string // optional: used by targets that support remote ruleset sections (e.g. QuanX)
	Rules   string
}

type RenderError struct {
	AppError model.AppError
	Cause    error
}

func (e *RenderError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.AppError.Code, e.AppError.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.AppError.Code, e.AppError.Message, e.Cause)
}

func (e *RenderError) Unwrap() error { return e.Cause }

func Render(target Target, res *compiler.Result) (Blocks, error) {
	if res == nil {
		return Blocks{}, &RenderError{
			AppError: model.AppError{
				Code:    "INVALID_ARGUMENT",
				Message: "render input 不能为空",
				Stage:   "render",
			},
		}
	}
	switch target {
	case TargetClash:
		return renderClash(res)
	case TargetSurge:
		return renderSurgeLike(res, true)
	case TargetShadowrocket:
		return renderSurgeLike(res, false)
	case TargetQuanx:
		return renderQuanx(res)
	default:
		return Blocks{}, &RenderError{
			AppError: model.AppError{
				Code:    "UNSUPPORTED_TARGET",
				Message: fmt.Sprintf("不支持的 target：%s", target),
				Stage:   "render",
			},
		}
	}
}
