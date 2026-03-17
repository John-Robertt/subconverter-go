package httpapi

import (
	"time"

	"github.com/John-Robertt/subconverter-go/internal/errlog"
)

// Options controls HTTP API runtime behavior (timeouts, etc.).
//
// Keep it small: this service is a compiler pipeline, not a framework.
type Options struct {
	// ConvertTimeout is the hard upper bound for a single conversion request
	// (fetch + parse + compile + render + template injection).
	ConvertTimeout time.Duration

	// FetchTimeout is the per-HTTP-request timeout used when fetching remote
	// resources (subscription/profile/template).
	FetchTimeout time.Duration

	// ErrorLog stores failure snapshots and exposes health/export operations.
	ErrorLog *errlog.Store
}

func (o Options) withDefaults() Options {
	if o.ConvertTimeout <= 0 {
		o.ConvertTimeout = 60 * time.Second
	}
	if o.FetchTimeout <= 0 {
		o.FetchTimeout = 15 * time.Second
	}
	return o
}
