package model

// AppError is the only error payload returned by this service in "strict mode".
// It is designed to match docs/spec/SPEC_HTTP_API.md.
type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Stage   string `json:"stage"`

	URL     string `json:"url,omitempty"`
	Line    int    `json:"line,omitempty"`    // 1-based; 0 means "not set"
	Snippet string `json:"snippet,omitempty"` // <= 200 chars recommended by spec
	Hint    string `json:"hint,omitempty"`
}

type ErrorResponse struct {
	Error AppError `json:"error"`
}
