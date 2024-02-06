package trace

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gitlab.com/efronlicht/enve"
)

// Trace is a pair of IDs that can be used to trace a request through the system.
// A TraceID is generated the first time Trace() is called on a request and transmitted across service boundaries via the X-Trace-ID header.
// A RequestID is generated when a client sends a request and transmitted to the server via the X-Request-ID header.
type Trace struct {
	TraceID, RequestID         string    // unique identifiers for the trace and request. requests are unique to a trace.
	TraceSource, RequestSource string    // the service that generated this trace or request
	TraceStart, RequestStart   time.Time // the time the trace was created and the time the request was received
}

// like http.ServeFunc, but for clients instead of servers.
type roundTripFunc func(*http.Request) (*http.Response, error)

// implement the http.RoundTripper interface
func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// ClientMiddleware wraps a RoundTripper, adding a Trace to each request's headers.
// It uses the trace in the request's context if it exists, or creates a new one if it doesn't.
//
// Example Usage:
//
//	http.DefaultClient.Transport = trace.ClientMiddleware(http.DefaultTransport)
//
// This middleware should be the first one executed in the chain, so that the Trace is available to all subsequent middlewares and handlers.
// Note that directly applied middlewares execute in Last-In, First-Out order, so this middleware should be the last one applied.
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
// This middleware should be the first one in the chain, so that the Trace is available to all subsequent middlewares and handlers.
// Note that directly applied middlewares execute in First-In, First-Out order, so this middleware should be the first one applied.
// Example Usage:
//
//	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("Hello, world!")) })
//	http.ListenAndServe(":8080", trace.ServerMiddleware(h))
func ServerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := FromHeaderOrNew(r.Header)
		ctx := CtxWith(r.Context(), t)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

var thisServiceName = enve.StringOr("RUNPOD_SERVICE_NAME", "unknown")

// New returns a new Trace with a new TraceID and RequestID and the current time as the TraceStart and RequestStart.
func New() Trace {
	now := time.Now().UTC()
	return Trace{
		TraceID:       newuuid(),
		RequestID:     newuuid(),
		TraceSource:   thisServiceName,
		RequestSource: thisServiceName,
		TraceStart:    now,
		RequestStart:  now,
	}
}

type ctxKey[T any] struct{}

// CtxWith returns a child context with the given Trace saved in it.
func CtxWith(ctx context.Context, t Trace) context.Context {
	return context.WithValue(ctx, ctxKey[Trace]{}, t)
}

// FromCtx returns the Trace from the given context, if it exists.
// If no Trace exists, the second return value is false, and it's your responsibility to inject a new one into the context.
func FromCtx(ctx context.Context) (t Trace, ok bool) {
	t, ok = ctx.Value(ctxKey[Trace]{}).(Trace)
	return t, ok
}

// / FromCtxOrNew returns the Trace from the given context, if it exists, and creates a new one if it doesn't.
func FromCtxOrNew(ctx context.Context) Trace {
	t, ok := FromCtx(ctx)
	if !ok {
		t = New()
	}
	return t
}

// Save a Trace into the given header, over-writing the X-Trace-ID, X-Request-ID, and X-Trace-Start headers.
// Note that there is no RequestStart header: the request timing starts when the server receives the request.
// This is in contrast to the TraceStart header, which is the time the trace was created and persists across service boundaries.
func SaveToHeader(h http.Header, t Trace) {
	h.Set("X-Trace-ID", t.TraceID)
	h.Set("X-Request-ID", t.RequestID)
	h.Set("X-Trace-Start", t.TraceStart.Format(time.RFC3339))
	h.Set("X-Trace-Source", t.TraceSource)
	h.Set("X-Request-Source", t.RequestSource)
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
	now := time.Now().UTC()

	var traceStart time.Time
	var err error
	if traceStart, err = time.Parse(time.RFC3339, h.Get("X-Trace-Start")); err != nil {
		traceStart = now
	}

	if traceStart.After(now) {
		slog.Warn("trace start is in the future", slog.Time("trace_start", traceStart), slog.Time("now", now))
		traceStart = now
	}

	return Trace{
		TraceID:       orelse(h.Get("X-Trace-ID"), newuuid),
		RequestID:     orelse(h.Get("X-Request-ID"), newuuid),
		TraceStart:    traceStart,
		RequestStart:  now,
		TraceSource:   h.Get("X-Trace-Source"),
		RequestSource: h.Get("X-Request-Source"),
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
