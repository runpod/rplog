// Package httpclient builds clients for a service's own outbound calls: one
// shared connection pool under RoundTrippers that set trace headers and a
// User-Agent. The caller supplies the User-Agent; nothing here is specific to
// a service.
//
// The transport in this library wraps http.DefaultTransport rather than
// replacing it, so every derived client shares the process-wide connection
// pool and libraries that clone http.DefaultTransport through a
// *http.Transport assertion (posthog-go) keep working.
package httpclient

import (
	"fmt"
	"net/http"
	"time"

	"github.com/runpod/rplog/trace"
)

const DefaultTimeout = 30 * time.Second

// UserAgent returns a User-Agent of the conventional "name/version" form, or
// just name when version is empty.
//
// Usually you won't use this directly, but will just call New() to get a client.
func UserAgent(name, version string) string {
	if version == "" {
		return name
	}
	return name + "/" + version
}

// validUserAgent reports whether s is a valid header value. Go rejects invalid
// values per request, so Transport checks once at construction instead.
// This is slightly more restrictive than Go, which accepts tab.
func validUserAgent(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < 0x20) || c == 0x7f {
			return false
		}
	}
	return true
}

// New returns the client to inject wherever a service makes outbound requests.
// It sends trace headers (see trace.ClientMiddleware) on every request, and
// UserAgent(name, version) on requests that don't set their own (empty leaves
// Go's default). It panics if it doesn't pass validUserAgent().
//
// This function assumes we don't need to create one without trace headers; if
// we ever do, we can make a NewWithoutTrace function.
func New(name, version string) *http.Client {
	return ForUserAgent(UserAgent(name, version))
}

// ForUserAgent is New with a complete User-Agent, for when it isn't of the
// "name/version" form. It panics if userAgent contains control characters.
func ForUserAgent(userAgent string) *http.Client {
	return &http.Client{
		Timeout:   DefaultTimeout,
		Transport: Transport(userAgent, nil),
	}
}

// Transport wraps base (http.DefaultTransport if nil) so requests carry trace
// headers and userAgent, as New's client does. Trace is outermost, so
// middlewares inside base see the trace headers; wrap logging in base, not
// around the result. It panics if userAgent contains control characters.
func Transport(userAgent string, base http.RoundTripper) http.RoundTripper {
	if !validUserAgent(userAgent) {
		panic(fmt.Sprintf("httpclient: invalid User-Agent %q", userAgent))
	}
	return trace.ClientMiddleware(userAgentTransport{userAgent: userAgent, base: base})
}

// WithTimeout returns a copy of base with timeout d.
//
// This function is NOT for setting timeouts for individual requests - use
// context.WithTimeout and http.NewRequestWithContext for that. The timeout
// here is a backstop for all timeouts made by the Client, for when ctx is
// omitted.
//
// Streaming and long-polling call sites can use this to widen the timeout. A
// timeout of 0 means no timeout - but you might just want a very long timeout
// instead.
//
// The copy shares base's Transport, and so its connection pool — building a
// client per call site is cheap; building a transport per call site
// would leak connections. A nil base means http.DefaultClient.
func WithTimeout(base *http.Client, timeout time.Duration) *http.Client {
	if base == nil {
		base = http.DefaultClient
	}
	c := *base
	c.Timeout = timeout
	return &c
}

var _ http.RoundTripper = userAgentTransport{}

// userAgentTransport sets userAgent on requests that don't already have one,
// then delegates to base (http.DefaultTransport if nil). An empty userAgent
// makes it a pass-through, leaving Go's default.
//
// Requests that already carry a User-Agent are left alone. That matters for a
// reverse proxy: an httputil.ReverseProxy using this as its Transport forwards
// the client's own User-Agent rather than replacing it with ours. Nothing in
// hapi proxies through it today, so it is unexported; export it if one does,
// rather than routing proxied traffic through New's client-level timeout,
// which would cut off long-lived streams.
type userAgentTransport struct {
	userAgent string
	base      http.RoundTripper
}

func (t userAgentTransport) transport() http.RoundTripper {
	if t.base != nil {
		return t.base
	}
	return http.DefaultTransport
}

func (t userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if _, ok := req.Header["User-Agent"]; !ok && t.userAgent != "" {
		// A RoundTripper must not mutate the request it's given - see docs for RoundTripper.
		req = req.Clone(req.Context())
		// Clone leaves a nil Header nil; only directly-constructed requests hit this.
		// This looks like user error, but assume this is equivalent to unset.
		if req.Header == nil {
			req.Header = make(http.Header)
		}
		req.Header.Set("User-Agent", t.userAgent)
	}
	return t.transport().RoundTrip(req)
}

// CloseIdleConnections forwards to the base transport so that
// (*http.Client).CloseIdleConnections is not a silent no-op.
func (t userAgentTransport) CloseIdleConnections() {
	if c, ok := t.transport().(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}
