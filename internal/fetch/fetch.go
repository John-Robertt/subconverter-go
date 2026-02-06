package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
	"unicode/utf8"

	"github.com/John-Robertt/subconverter-go/internal/model"
)

type Kind int

const (
	KindSubscription Kind = iota
	KindProfile
	KindTemplate
)

func (k Kind) stage() string {
	switch k {
	case KindSubscription:
		return "fetch_sub"
	case KindProfile:
		return "fetch_profile"
	case KindTemplate:
		return "fetch_template"
	default:
		// Unknown kind is a programmer error; still return something stable.
		return "fetch"
	}
}

func (k Kind) defaultMaxBytes() int64 {
	// Defaults from docs/spec/SPEC_FETCH.md.
	switch k {
	case KindSubscription:
		return 5 * 1024 * 1024
	case KindProfile:
		return 1 * 1024 * 1024
	case KindTemplate:
		return 2 * 1024 * 1024
	default:
		return 1 * 1024 * 1024
	}
}

type Options struct {
	Timeout      time.Duration // default 15s
	MaxBytes     int64         // default per kind
	MaxRedirects int           // default 5
}

type FetchError struct {
	Status   int
	AppError model.AppError
	Cause    error
}

func (e *FetchError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.AppError.Code, e.AppError.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.AppError.Code, e.AppError.Message, e.Cause)
}

func (e *FetchError) Unwrap() error { return e.Cause }

var (
	errTooManyRedirects   = errors.New("too many redirects")
	errRedirectBadScheme  = errors.New("redirect target scheme is not http/https")
	errInvalidURLOrScheme = errors.New("invalid url or scheme")
)

func FetchText(ctx context.Context, kind Kind, rawURL string) (string, error) {
	return FetchTextWithOptions(ctx, kind, rawURL, Options{})
}

func FetchTextWithOptions(ctx context.Context, kind Kind, rawURL string, opt Options) (string, error) {
	stage := kind.stage()

	timeout := opt.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	maxRedirects := opt.MaxRedirects
	if maxRedirects == 0 {
		maxRedirects = 5
	}
	maxBytes := opt.MaxBytes
	if maxBytes == 0 {
		maxBytes = kind.defaultMaxBytes()
	}
	if maxBytes <= 0 {
		return "", &FetchError{
			Status: http.StatusBadRequest,
			AppError: model.AppError{
				Code:    "INVALID_ARGUMENT",
				Message: "响应大小上限必须大于 0",
				Stage:   stage,
				URL:     rawURL,
			},
		}
	}

	u, err := url.Parse(rawURL)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", &FetchError{
			Status: http.StatusBadRequest,
			AppError: model.AppError{
				Code:    "INVALID_ARGUMENT",
				Message: "仅允许 http/https URL",
				Stage:   stage,
				URL:     rawURL,
			},
			Cause: errors.Join(errInvalidURLOrScheme, err),
		}
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: http.DefaultTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// via is already the chain of previous requests; allow up to maxRedirects redirects.
			// 1st redirect => len(via)==1, 5th redirect => len(via)==5.
			if len(via) > maxRedirects {
				return errTooManyRedirects
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errRedirectBadScheme
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		// Should be rare after url.Parse succeeded, but keep the error explicit.
		return "", &FetchError{
			Status: http.StatusBadRequest,
			AppError: model.AppError{
				Code:    "INVALID_ARGUMENT",
				Message: "请求 URL 不合法",
				Stage:   stage,
				URL:     rawURL,
			},
			Cause: err,
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}

		// CheckRedirect sentinel errors.
		if errors.Is(err, errTooManyRedirects) {
			return "", &FetchError{
				Status: http.StatusBadGateway,
				AppError: model.AppError{
					Code:    "FETCH_FAILED",
					Message: fmt.Sprintf("重定向次数超过上限（>%d）", maxRedirects),
					Stage:   stage,
					URL:     rawURL,
				},
				Cause: err,
			}
		}
		if errors.Is(err, errRedirectBadScheme) {
			return "", &FetchError{
				Status: http.StatusBadRequest,
				AppError: model.AppError{
					Code:    "INVALID_ARGUMENT",
					Message: "重定向目标仅允许 http/https",
					Stage:   stage,
					URL:     rawURL,
				},
				Cause: err,
			}
		}

		// Timeout detection: Go may wrap errors (e.g. *url.Error).
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return "", &FetchError{
				Status: http.StatusGatewayTimeout,
				AppError: model.AppError{
					Code:    "FETCH_TIMEOUT",
					Message: "拉取远程资源超时",
					Stage:   stage,
					URL:     rawURL,
				},
				Cause: err,
			}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return "", &FetchError{
				Status: http.StatusGatewayTimeout,
				AppError: model.AppError{
					Code:    "FETCH_TIMEOUT",
					Message: "拉取远程资源超时",
					Stage:   stage,
					URL:     rawURL,
				},
				Cause: err,
			}
		}

		return "", &FetchError{
			Status: http.StatusBadGateway,
			AppError: model.AppError{
				Code:    "FETCH_FAILED",
				Message: "拉取远程资源失败",
				Stage:   stage,
				URL:     rawURL,
			},
			Cause: err,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &FetchError{
			Status: http.StatusBadGateway,
			AppError: model.AppError{
				Code:    "FETCH_FAILED",
				Message: fmt.Sprintf("上游返回非 2xx 状态码：%d", resp.StatusCode),
				Stage:   stage,
				URL:     rawURL,
			},
		}
	}

	// Read at most maxBytes+1 to detect overflow deterministically.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return "", &FetchError{
				Status: http.StatusGatewayTimeout,
				AppError: model.AppError{
					Code:    "FETCH_TIMEOUT",
					Message: "拉取远程资源超时",
					Stage:   stage,
					URL:     rawURL,
				},
				Cause: err,
			}
		}
		return "", &FetchError{
			Status: http.StatusBadGateway,
			AppError: model.AppError{
				Code:    "FETCH_FAILED",
				Message: "读取上游响应失败",
				Stage:   stage,
				URL:     rawURL,
			},
			Cause: err,
		}
	}
	if int64(len(body)) > maxBytes {
		return "", &FetchError{
			Status: http.StatusUnprocessableEntity,
			AppError: model.AppError{
				Code:    "TOO_LARGE",
				Message: fmt.Sprintf("远程资源过大（>%d bytes）", maxBytes),
				Stage:   stage,
				URL:     rawURL,
			},
		}
	}
	if !utf8.Valid(body) {
		return "", &FetchError{
			Status: http.StatusUnprocessableEntity,
			AppError: model.AppError{
				Code:    "FETCH_INVALID_UTF8",
				Message: "远程资源不是合法 UTF-8 文本",
				Stage:   stage,
				URL:     rawURL,
			},
		}
	}

	return string(body), nil
}
