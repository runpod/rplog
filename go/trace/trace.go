package trace

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// Trace is a pair of IDs that can be used to trace a request through the system.
// A TraceID is generated the first time Trace() is called on a request and transmitted across service boundaries via the X-Trace-ID header.
// A RequestID is generated when a client sends a request and transmitted to the server via the X-Request-ID header.
type Trace struct {
	TraceID   string
	RequestID string
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// ClientMiddleware wraps a RoundTripper, adding a Trace to each request's headers.
// It uses the trace in the request's context if it exists, or creates a new one if it doesn't.
func ClientMiddleware(rt http.RoundTripper) http.RoundTripper {
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		// check if the request already has a trace. If not, create a new one.
		t, ok := FromCtx(r.Context())
		if !ok {
			t = New()
		} else { // make a new request ID for this sub-request before shoving it across the wire
			t.RequestID = newuuid()
		}
		SaveToHeader(r.Header, t)
		r = r.WithContext(CtxWith(r.Context(), t))
		return rt.RoundTrip(r)
	})
}

// ServerMiddleware adds a Trace to the request's context before passing it to the next handler.
func ServerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := FromHeaderOrNew(r.Header)
		ctx := CtxWith(r.Context(), t)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// New starts a brand new Trace.
func New() Trace { return Trace{TraceID: newuuid(), RequestID: newuuid()} }

type ctxKey[T any] struct{}

// CtxWith returns a child context with the given Trace saved in it.
func CtxWith(ctx context.Context, t Trace) context.Context {
	return context.WithValue(ctx, ctxKey[Trace]{}, t)
}

// FromCtx returns the Trace from the given context, if it exists.
// If no Trace exists, the second return value is false, and it's your responsibility to inject a new one into the context.
func FromCtx(ctx context.Context) (t Trace, ok bool) {
	t, ok = ctx.Value(ctxKey[Trace]{}).(Trace)
	return
}

// Save a Trace into the given header, over-writing the X-Trace-ID and X-Request-ID fields.
func SaveToHeader(h http.Header, t Trace) {
	h.Set("X-Trace-ID", t.TraceID)
	h.Set("X-Request-ID", t.RequestID)
}

// uuid generates a new UUID, preferring V7 over V4, but falling back to V4 if V7 is not available.
func newuuid() string {
	u, err := uuid.NewV7()
	if err != nil {
		u = uuid.New()
	}
	return u.String()
}

// FromHeaderOrNew returns a Trace from the given header, if it exists, and creates a new one if it doesn't.
func FromHeaderOrNew(h http.Header) Trace {
	return Trace{
		TraceID:   orelse(h.Get("X-Trace-ID"), newuuid),
		RequestID: orelse(h.Get("X-Request-ID"), newuuid),
	}
}

// return a if it's non-zero, otherwise call f and return its result.
func orelse[T comparable](a T, f func() T) T {
	var zero T
	if a == zero {
		return f()
	}
	return a
}
