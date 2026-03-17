package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

type requestIDKey struct{}

var requestSeq atomic.Uint64

func withRequestID(r *http.Request) *http.Request {
	if r == nil {
		return nil
	}
	if id := requestIDFromContext(r.Context()); id != "" {
		return r
	}
	id := nextRequestID()
	ctx := context.WithValue(r.Context(), requestIDKey{}, id)
	return r.WithContext(ctx)
}

func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(requestIDKey{}).(string)
	return v
}

func ensureRequestID(r *http.Request) string {
	if r == nil {
		return nextRequestID()
	}
	if id := requestIDFromContext(r.Context()); id != "" {
		return id
	}
	return nextRequestID()
}

func nextRequestID() string {
	seq := requestSeq.Add(1)
	return fmt.Sprintf("req-%d-%d", time.Now().UnixNano(), seq)
}
