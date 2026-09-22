package trace

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Shared fixtures, hoisted so the sample ids and source aren't repeated literals
// across the tables (keeps goconst quiet and the intent obvious).
const (
	sampleTraceID   = "5f9c2e6a-1b3d-4c8e-9a0f-2b7c6d5e4f31"
	sampleRequestID = "018f3a1c-2b4d-7e8f-9a0b-1c2d3e4f5061"
	sampleSource    = "runpod-graphql"
)

func TestValidID(t *testing.T) {
	tests := []struct {
		description string
		input       string
		expected    bool
	}{
		{description: "canonical uuid v4", input: sampleTraceID, expected: true},
		{description: "uuid v7 style", input: sampleRequestID, expected: true},
		{description: "service-prefixed id with underscore", input: "trace_018f3a1c2b4d7e8f", expected: true},
		{description: "dotted id", input: "svc.abc.123", expected: true},
		{description: "all allowed classes", input: "AZaz09-_.", expected: true},
		{description: "exactly maxIDLen chars", input: strings.Repeat("a", maxIDLen), expected: true},
		{description: "empty string is rejected", input: "", expected: false},
		{description: "one over maxIDLen is rejected", input: strings.Repeat("a", maxIDLen+1), expected: false},
		{description: "embedded CRLF (header/log injection) is rejected", input: "abc\r\nX-Evil: 1", expected: false},
		{description: "bare newline is rejected", input: "abc\ndef", expected: false},
		{description: "space is rejected", input: "abc def", expected: false},
		{description: "control byte is rejected", input: "abc\x00def", expected: false},
		{description: "non-ascii is rejected", input: "abcé", expected: false},
		{description: "slash is rejected", input: "abc/def", expected: false},
		{description: "colon is rejected", input: "abc:def", expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			if got := validID(tt.input); got != tt.expected {
				t.Errorf("validID(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestResolveID(t *testing.T) {
	tests := []struct {
		description   string
		input         string
		wantPropagate bool // true: returned verbatim; false: freshly generated
	}{
		{description: "valid id is propagated verbatim", input: sampleTraceID, wantPropagate: true},
		{description: "absent id is generated", input: "", wantPropagate: false},
		{description: "oversized id is regenerated", input: strings.Repeat("a", maxIDLen+1), wantPropagate: false},
		{description: "CRLF id is regenerated", input: "abc\r\nInjected: 1", wantPropagate: false},
		{description: "junk id is regenerated", input: "abc def", wantPropagate: false},
	}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			got := resolveID(tt.input)
			if tt.wantPropagate {
				if got != tt.input {
					t.Errorf("resolveID(%q) = %q, want it propagated verbatim", tt.input, got)
				}
				return
			}
			if got == tt.input {
				t.Errorf("resolveID(%q) returned the input; expected a fresh id", tt.input)
			}
			if !validID(got) {
				t.Errorf("resolveID(%q) generated an invalid id %q", tt.input, got)
			}
		})
	}
}

func TestResolveSource(t *testing.T) {
	tests := []struct {
		description string
		input       string
		expected    string
	}{
		{description: "empty source stays empty", input: "", expected: ""},
		{description: "valid service name is kept", input: sampleSource, expected: sampleSource},
		{description: "CRLF source is dropped to empty", input: "svc\r\nX-Evil: 1", expected: ""},
		{description: "spaced source is dropped to empty", input: "not a slug", expected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			if got := resolveSource(tt.input); got != tt.expected {
				t.Errorf("resolveSource(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFromHeaderOrNew(t *testing.T) {
	const goodTrace = sampleTraceID
	const goodReq = sampleRequestID

	tests := []struct {
		description       string
		headers           map[string]string
		wantTraceID       string // "" => expect a generated valid id
		wantRequestID     string // "" => expect a generated valid id
		wantTraceSource   string
		wantRequestSource string
	}{
		{
			description:       "all ids present and valid are propagated",
			headers:           map[string]string{headerTraceID: goodTrace, headerRequestID: goodReq, headerTraceSource: "main-ui", headerRequestSource: "hapi"},
			wantTraceID:       goodTrace,
			wantRequestID:     goodReq,
			wantTraceSource:   "main-ui",
			wantRequestSource: "hapi",
		},
		{
			description:   "no headers generates both ids",
			headers:       map[string]string{},
			wantTraceID:   "",
			wantRequestID: "",
		},
		{
			description:   "poisoned trace id is regenerated, valid request id kept",
			headers:       map[string]string{headerTraceID: "abc\r\nX-Evil: 1", headerRequestID: goodReq},
			wantTraceID:   "",
			wantRequestID: goodReq,
		},
		{
			description:     "poisoned source is dropped to empty",
			headers:         map[string]string{headerTraceID: goodTrace, headerRequestID: goodReq, headerTraceSource: "svc\r\nX-Evil: 1"},
			wantTraceID:     goodTrace,
			wantRequestID:   goodReq,
			wantTraceSource: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			h := http.Header{}
			for k, v := range tt.headers {
				h.Set(k, v)
			}
			got := FromHeaderOrNew(h)

			assertID := func(name, want, actual string) {
				if want == "" {
					if actual == tt.headers[name] && tt.headers[name] != "" {
						t.Errorf("%s = %q, expected a regenerated id", name, actual)
					}
					if !validID(actual) {
						t.Errorf("%s = %q is not a valid generated id", name, actual)
					}
					return
				}
				if actual != want {
					t.Errorf("%s = %q, want %q", name, actual, want)
				}
			}
			assertID(headerTraceID, tt.wantTraceID, got.TraceID)
			assertID(headerRequestID, tt.wantRequestID, got.RequestID)
			if got.TraceSource != tt.wantTraceSource {
				t.Errorf("TraceSource = %q, want %q", got.TraceSource, tt.wantTraceSource)
			}
			if got.RequestSource != tt.wantRequestSource {
				t.Errorf("RequestSource = %q, want %q", got.RequestSource, tt.wantRequestSource)
			}
		})
	}
}

func TestFromHeaderOrNewTraceStart(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		description string
		header      string
		expected    func(got time.Time) bool
	}{
		{
			description: "absent trace-start defaults to ~now",
			header:      "",
			expected: func(got time.Time) bool {
				return !got.After(now.Add(time.Second)) && !got.Before(now.Add(-time.Second))
			},
		},
		{
			description: "valid past trace-start is preserved",
			header:      "2020-01-01T00:00:00Z",
			expected: func(got time.Time) bool {
				want, _ := time.Parse(time.RFC3339, "2020-01-01T00:00:00Z")
				return got.Equal(want)
			},
		},
		{
			description: "invalid trace-start falls back to ~now",
			header:      "not-a-timestamp",
			expected:    func(got time.Time) bool { return !got.Before(now.Add(-time.Second)) },
		},
		{
			description: "future trace-start is clamped to ~now",
			header:      now.Add(time.Hour).Format(time.RFC3339),
			expected:    func(got time.Time) bool { return !got.After(now.Add(time.Second)) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			h := http.Header{}
			if tt.header != "" {
				h.Set(headerTraceStart, tt.header)
			}
			got := FromHeaderOrNew(h)
			if !tt.expected(got.TraceStart) {
				t.Errorf("TraceStart = %v not within expectation for %q", got.TraceStart, tt.header)
			}
		})
	}
}

func TestSaveToHeaderRoundTrip(t *testing.T) {
	orig := Trace{
		TraceID:       sampleTraceID,
		RequestID:     sampleRequestID,
		TraceSource:   sampleSource,
		RequestSource: sampleSource,
		TraceStart:    time.Now().UTC().Truncate(time.Second),
	}
	h := http.Header{}
	SaveToHeader(h, orig)

	got := FromHeaderOrNew(h)
	if got.TraceID != orig.TraceID || got.RequestID != orig.RequestID {
		t.Errorf("round-trip ids: got trace=%q req=%q, want trace=%q req=%q", got.TraceID, got.RequestID, orig.TraceID, orig.RequestID)
	}
	if got.TraceSource != orig.TraceSource {
		t.Errorf("round-trip TraceSource = %q, want %q", got.TraceSource, orig.TraceSource)
	}
	if !got.TraceStart.Equal(orig.TraceStart) {
		t.Errorf("round-trip TraceStart = %v, want %v", got.TraceStart, orig.TraceStart)
	}
}

func TestSaveToHeaderEmitsNoUnsafeBytes(t *testing.T) {
	// A Trace built from a poisoned inbound header must never re-emit CR/LF (or
	// other control bytes) through SaveToHeader — otherwise the outbound request
	// fails or smuggles a header downstream.
	poisoned := http.Header{}
	poisoned.Set(headerTraceID, "abc\r\nX-Evil: 1")
	poisoned.Set(headerRequestID, "req\r\nX-Evil: 2")
	poisoned.Set(headerTraceSource, "svc\r\nX-Evil: 3")
	trc := FromHeaderOrNew(poisoned)

	out := http.Header{}
	SaveToHeader(out, trc)
	for key, values := range out {
		for _, v := range values {
			if strings.ContainsAny(v, "\r\n\x00") {
				t.Errorf("header %q carries unsafe bytes after SaveToHeader: %q", key, v)
			}
		}
	}
}

type capturingRT struct{ req *http.Request }

func (c *capturingRT) RoundTrip(r *http.Request) (*http.Response, error) {
	c.req = r
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestClientMiddleware(t *testing.T) {
	t.Run("reuses trace id and mints a new request id when ctx has a trace", func(t *testing.T) {
		parent := Trace{TraceID: "trace-abc", RequestID: "req-parent", TraceSource: "svc"}
		cap := &capturingRT{}
		rt := ClientMiddleware(cap)

		req, _ := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
		req = req.WithContext(CtxWith(req.Context(), parent))
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
		resp.Body.Close()
		if got := cap.req.Header.Get(headerTraceID); got != "trace-abc" {
			t.Errorf("X-Trace-ID = %q, want the parent trace id", got)
		}
		if got := cap.req.Header.Get(headerRequestID); got == "" || got == "req-parent" {
			t.Errorf("X-Request-ID = %q, want a fresh sub-request id", got)
		}
	})

	t.Run("creates a fresh trace when ctx has none", func(t *testing.T) {
		cap := &capturingRT{}
		rt := ClientMiddleware(cap)
		req, _ := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
		resp.Body.Close()
		if got := cap.req.Header.Get(headerTraceID); !validID(got) {
			t.Errorf("X-Trace-ID = %q, want a generated valid id", got)
		}
	})
}

func TestServerMiddlewarePutsTraceInContext(t *testing.T) {
	var seen Trace
	var ok bool
	h := ServerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, ok = FromCtx(r.Context())
	}))
	req, _ := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	req.Header.Set(headerTraceID, "trace-from-header")
	h.ServeHTTP(nil, req)
	if !ok {
		t.Fatal("expected a Trace in the request context")
	}
	if seen.TraceID != "trace-from-header" {
		t.Errorf("TraceID = %q, want %q", seen.TraceID, "trace-from-header")
	}
}

func TestNewGeneratesUniqueIDs(t *testing.T) {
	const n = 10_000
	traces := make(map[string]struct{}, n)
	reqs := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		tr := New()
		if _, dup := traces[tr.TraceID]; dup {
			t.Fatalf("duplicate TraceID at i=%d: %q", i, tr.TraceID)
		}
		if _, dup := reqs[tr.RequestID]; dup {
			t.Fatalf("duplicate RequestID at i=%d: %q", i, tr.RequestID)
		}
		traces[tr.TraceID] = struct{}{}
		reqs[tr.RequestID] = struct{}{}
	}
}

func TestValidIDIsAllocationFree(t *testing.T) {
	input := sampleTraceID
	if allocs := testing.AllocsPerRun(1000, func() { _ = validID(input) }); allocs != 0 {
		t.Errorf("validID allocated %v times per run, want 0", allocs)
	}
}

func BenchmarkFromHeaderOrNew(b *testing.B) {
	valid := http.Header{}
	valid.Set(headerTraceID, sampleTraceID)
	valid.Set(headerRequestID, sampleRequestID)

	invalid := http.Header{}
	invalid.Set(headerTraceID, "abc\r\nX-Evil: 1")

	absent := http.Header{}

	cases := map[string]http.Header{"present": valid, "absent": absent, "invalid": invalid}
	for name, h := range cases {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = FromHeaderOrNew(h)
			}
		})
	}
}

func BenchmarkValidID(b *testing.B) {
	input := sampleTraceID
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = validID(input)
	}
}

func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = New()
	}
}

func BenchmarkNewUUID(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = newuuid()
	}
}

func BenchmarkSaveToHeader(b *testing.B) {
	trc := New()
	h := http.Header{}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		SaveToHeader(h, trc)
	}
}

func BenchmarkClientMiddlewareRoundTrip(b *testing.B) {
	rt := ClientMiddleware(&capturingRT{})
	req, _ := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	req = req.WithContext(CtxWith(context.Background(), New()))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, _ := rt.RoundTrip(req)
		resp.Body.Close()
	}
}
