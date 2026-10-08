package httpclient

import (
	"net/http"
	"testing"
)

// nopTransport stands in for the real base so the benchmarks measure only the
// wrapper, not the network.
type nopTransport struct{ resp *http.Response }

func (n nopTransport) RoundTrip(*http.Request) (*http.Response, error) { return n.resp, nil }

func benchRequest(b *testing.B, ua string) *http.Request {
	req, err := http.NewRequest(http.MethodGet, "http://example.invalid/v1/pod", nil)
	if err != nil {
		b.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	return req
}

func run(b *testing.B, rt http.RoundTripper, req *http.Request) {
	b.ReportAllocs()
	for b.Loop() {
		resp, err := rt.RoundTrip(req)
		if err != nil {
			b.Fatal(err)
		}
		_ = resp
	}
}

// Baseline: the base transport alone. Subtract this from the others to get the
// wrapper's overhead.
func BenchmarkBase(b *testing.B) {
	base := nopTransport{resp: &http.Response{StatusCode: http.StatusOK}}
	run(b, base, benchRequest(b, ""))
}

// Worst case: the header is missing, so the wrapper clones the request.
func BenchmarkTransportUnset(b *testing.B) {
	rt := userAgentTransport{userAgent: "hapiproxy/1.33.0", base: nopTransport{resp: &http.Response{StatusCode: http.StatusOK}}}
	run(b, rt, benchRequest(b, ""))
}

// Pass-through case: the caller set a User-Agent, so no clone happens.
func BenchmarkTransportPreset(b *testing.B) {
	rt := userAgentTransport{userAgent: "hapiproxy/1.33.0", base: nopTransport{resp: &http.Response{StatusCode: http.StatusOK}}}
	run(b, rt, benchRequest(b, "caller/1.0"))
}
