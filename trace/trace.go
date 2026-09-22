package trace

import (
	"context"
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

// Trace headers carried across service boundaries. Only X-Trace-ID and
// X-Request-ID are required for interop; the rest are metadata.
const (
	headerTraceID       = "X-Trace-ID"
	headerRequestID     = "X-Request-ID"
	headerTraceStart    = "X-Trace-Start"
	headerTraceSource   = "X-Trace-Source"
	headerRequestSource = "X-Request-Source"
)

// Save a Trace into the given header, over-writing the X-Trace-ID, X-Request-ID, and X-Trace-Start headers.
// Note that there is no RequestStart header: the request timing starts when the server receives the request.
// This is in contrast to the TraceStart header, which is the time the trace was created and persists across service boundaries.
func SaveToHeader(h http.Header, t Trace) {
	h.Set(headerTraceID, t.TraceID)
	h.Set(headerRequestID, t.RequestID)
	h.Set(headerTraceStart, t.TraceStart.Format(time.RFC3339))
	h.Set(headerTraceSource, t.TraceSource)
	h.Set(headerRequestSource, t.RequestSource)
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
//
// Inbound header values are untrusted. A TraceID/RequestID that is empty, over
// maxIDLen, or carries bytes outside [A-Za-z0-9._-] is replaced with a fresh
// uuid, and an out-of-charset TraceSource/RequestSource is dropped to "". This
// keeps every value SaveToHeader later re-emits safe to write into an HTTP
// header: a CR/LF-bearing id would otherwise make the outbound request (or the
// response echo) fail, or smuggle a header into a lenient downstream.
func FromHeaderOrNew(h http.Header) Trace {
	now := time.Now().UTC()

	// X-Trace-Start is caller-controlled and untrusted. Parse it, but fall back to
	// now for absent, malformed, or future values. A future timestamp is clamped
	// silently, exactly like a malformed one, rather than logged: warning per
	// request would let a caller drive this service's log volume by choosing the
	// header value.
	traceStart := now
	if raw := h.Get(headerTraceStart); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil && !parsed.After(now) {
			traceStart = parsed
		}
	}

	return Trace{
		TraceID:       resolveID(h.Get(headerTraceID)),
		RequestID:     resolveID(h.Get(headerRequestID)),
		TraceStart:    traceStart,
		RequestStart:  now,
		TraceSource:   resolveSource(h.Get(headerTraceSource)),
		RequestSource: resolveSource(h.Get(headerRequestSource)),
	}
}

// maxIDLen caps how long an inbound trace/request id may be before we treat it
// as untrustworthy and mint a fresh one.
const maxIDLen = 200

// validID reports whether s is safe to accept verbatim from an untrusted
// inbound header: non-empty, within maxIDLen, and limited to characters that
// cannot corrupt a log line or an HTTP header value (no CR/LF, control bytes,
// spaces, or non-ASCII). The scan indexes bytes and allocates nothing.
func validID(s string) bool {
	if s == "" || len(s) > maxIDLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}

// resolveID returns id when it passes validID, otherwise a fresh uuid.
func resolveID(id string) string {
	if validID(id) {
		return id
	}
	return newuuid()
}

// resolveSource returns src unchanged when empty (the "unknown" case) or when
// it passes validID, and drops any other value to "" so SaveToHeader can never
// re-emit unsafe bytes.
func resolveSource(src string) string {
	if src == "" || validID(src) {
		return src
	}
	return ""
}
