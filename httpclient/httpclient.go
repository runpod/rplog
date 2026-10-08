// Package httpclient builds clients for a service's own outbound calls: one
// shared connection pool under a User-Agent-setting RoundTripper. The caller
// supplies the User-Agent; nothing here is specific to a service.
package httpclient

import (
	"net/http"
	"time"
)

// DefaultTimeout bounds any request whose call site doesn't override it. It is
// a backstop against a hung upstream, not a latency budget — call sites that
// need a real deadline set their own with WithTimeout.
const DefaultTimeout = 30 * time.Second

// UserAgent returns a User-Agent of the conventional "name/version" form, or
// just name when version is empty.
func UserAgent(name, version string) string {
	if version == "" {
		return name
	}
	return name + "/" + version
}

// New returns the client to inject wherever a service makes outbound requests.
// It sends userAgent on requests that don't set their own (empty leaves Go's
// default). Its transport wraps http.DefaultTransport rather than replacing it,
// so every derived client shares the process-wide connection pool and
// libraries that clone http.DefaultTransport through a *http.Transport
// assertion (posthog-go) keep working.
func New(userAgent string) *http.Client {
	return &http.Client{
		Timeout:   DefaultTimeout,
		Transport: userAgentTransport{userAgent: userAgent, base: http.DefaultTransport},
	}
}

// WithTimeout returns a copy of base with timeout d; d == 0 means no timeout,
// which is what streaming call sites want. The copy shares base's Transport,
// and so its connection pool — building a client per call site is cheap,
// building a transport per call site would leak connections.
func WithTimeout(base *http.Client, d time.Duration) *http.Client {
	c := *base
	c.Timeout = d
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

func (t userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if _, ok := req.Header["User-Agent"]; !ok && t.userAgent != "" {
		// A RoundTripper must not mutate the request it's given.
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", t.userAgent)
	}
	return base.RoundTrip(req)
}
