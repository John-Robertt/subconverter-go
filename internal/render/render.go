package render

import (
	"fmt"
	"strings"

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
	Proxies       string
	Groups        string
	RuleProviders string // optional: used by Clash rule-providers
	Rulesets      string // optional: used by targets that support remote ruleset sections (e.g. QuanX)
	Rules         string
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
	if err := validateRenderInput(res); err != nil {
		return Blocks{}, err
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

func validateRenderInput(res *compiler.Result) error {
	// Renderers resolve typed group members by proxy ID. Fail fast here so direct
	// callers get a clear contract error instead of renderer-specific fallout.
	seenProxyIDs := make(map[string]struct{}, len(res.Proxies))
	for i, p := range res.Proxies {
		if strings.TrimSpace(p.ID) == "" {
			return &RenderError{
				AppError: model.AppError{
					Code:    "INVALID_ARGUMENT",
					Message: "render input 的 proxy ID 不能为空",
					Stage:   "render",
					Snippet: fmt.Sprintf("proxies[%d]: %s", i, p.Name),
				},
			}
		}
		if _, ok := seenProxyIDs[p.ID]; ok {
			return &RenderError{
				AppError: model.AppError{
					Code:    "INVALID_ARGUMENT",
					Message: "render input 的 proxy ID 必须唯一",
					Stage:   "render",
					Snippet: p.ID,
				},
			}
		}
		seenProxyIDs[p.ID] = struct{}{}
	}
	return nil
}
