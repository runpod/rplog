package trace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// mustUUID fails the test if s is not a well-formed UUID.
func mustUUID(t *testing.T, field, s string) {
	t.Helper()
	if _, err := uuid.Parse(s); err != nil {
		t.Errorf("%s = %q is not a valid UUID: %v", field, s, err)
	}
}

func TestNew(t *testing.T) {
	before := time.Now().UTC()
	tr := New()
	after := time.Now().UTC()

	mustUUID(t, "TraceID", tr.TraceID)
	mustUUID(t, "RequestID", tr.RequestID)
	if tr.TraceID == tr.RequestID {
		t.Error("TraceID and RequestID should differ")
	}
	if tr.TraceSource != thisServiceName || tr.RequestSource != thisServiceName {
		t.Errorf("sources = %q/%q, want %q", tr.TraceSource, tr.RequestSource, thisServiceName)
	}
	if tr.TraceStart.Before(before) || tr.TraceStart.After(after) {
		t.Errorf("TraceStart %v not within [%v, %v]", tr.TraceStart, before, after)
	}
	if !tr.TraceStart.Equal(tr.RequestStart) {
		t.Errorf("New should set TraceStart == RequestStart, got %v vs %v", tr.TraceStart, tr.RequestStart)
	}
}

func TestNewUUID(t *testing.T) {
	// positive: many calls are all valid and unique
	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		u := newuuid()
		mustUUID(t, "newuuid", u)
		if _, dup := seen[u]; dup {
			t.Fatalf("newuuid produced a duplicate: %q", u)
		}
		seen[u] = struct{}{}
	}
}

func TestCtxRoundTrip(t *testing.T) {
	// negative: empty context has no trace
	if _, ok := FromCtx(context.Background()); ok {
		t.Error("FromCtx on empty context should report ok=false")
	}

	// positive: value stored is the value retrieved
	tr := New()
	ctx := CtxWith(context.Background(), tr)
	got, ok := FromCtx(ctx)
	if !ok {
		t.Fatal("FromCtx should find the stored trace")
	}
	if got != tr {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, tr)
	}

	// corner: FromCtxOrNew mints a fresh valid trace when none present
	minted := FromCtxOrNew(context.Background())
	mustUUID(t, "FromCtxOrNew.TraceID", minted.TraceID)
	// and returns the existing one when present
	if again := FromCtxOrNew(ctx); again != tr {
		t.Errorf("FromCtxOrNew should return existing trace, got %+v", again)
	}
}

func TestSaveToHeaderRoundTrip(t *testing.T) {
	tr := Trace{
		TraceID:       newuuid(),
		RequestID:     newuuid(),
		TraceSource:   "svc-a",
		RequestSource: "svc-b",
		TraceStart:    time.Now().UTC().Add(-time.Minute).Truncate(time.Second),
	}
	h := http.Header{}
	SaveToHeader(h, tr)

	got := FromHeaderOrNew(h)
	if got.TraceID != tr.TraceID || got.RequestID != tr.RequestID {
		t.Errorf("id round-trip failed: got %q/%q want %q/%q", got.TraceID, got.RequestID, tr.TraceID, tr.RequestID)
	}
	if got.TraceSource != tr.TraceSource || got.RequestSource != tr.RequestSource {
		t.Errorf("source round-trip failed: got %q/%q want %q/%q", got.TraceSource, got.RequestSource, tr.TraceSource, tr.RequestSource)
	}
	if !got.TraceStart.Equal(tr.TraceStart) {
		t.Errorf("TraceStart round-trip failed: got %v want %v", got.TraceStart, tr.TraceStart)
	}
}

func TestFromHeaderOrNew(t *testing.T) {
	validTrace := "018f8c1e-0000-7000-8000-000000000001"
	validReq := "018f8c1e-0000-7000-8000-000000000002"
	pastStart := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)

	type tc struct {
		name     string
		class    string
		header   http.Header
		validate func(t *testing.T, got Trace)
	}
	cases := []tc{
		{
			name:  "well-formed headers preserved",
			class: "positive",
			header: http.Header{
				"X-Trace-Id":     {validTrace},
				"X-Request-Id":   {validReq},
				"X-Trace-Start":  {pastStart.Format(time.RFC3339)},
				"X-Trace-Source": {"gateway"},
			},
			validate: func(t *testing.T, got Trace) {
				if got.TraceID != validTrace || got.RequestID != validReq {
					t.Errorf("ids not preserved: %q/%q", got.TraceID, got.RequestID)
				}
				if !got.TraceStart.Equal(pastStart) {
					t.Errorf("TraceStart not preserved: got %v want %v", got.TraceStart, pastStart)
				}
				if got.TraceSource != "gateway" {
					t.Errorf("TraceSource = %q", got.TraceSource)
				}
			},
		},
		{
			name:   "missing headers generate fresh trace",
			class:  "negative",
			header: http.Header{},
			validate: func(t *testing.T, got Trace) {
				mustUUID(t, "TraceID", got.TraceID)
				mustUUID(t, "RequestID", got.RequestID)
				if got.TraceSource != "" || got.RequestSource != "" {
					t.Errorf("empty sources expected, got %q/%q", got.TraceSource, got.RequestSource)
				}
			},
		},
		{
			name:  "trace start in the future is clamped to now",
			class: "boundary",
			header: http.Header{
				"X-Trace-Start": {time.Now().UTC().Add(time.Hour).Format(time.RFC3339)},
			},
			validate: func(t *testing.T, got Trace) {
				if got.TraceStart.After(time.Now().UTC().Add(time.Second)) {
					t.Errorf("future TraceStart not clamped: %v", got.TraceStart)
				}
			},
		},
		{
			name:  "malformed trace start falls back to now",
			class: "corner",
			header: http.Header{
				"X-Trace-Start": {"not-a-timestamp"},
			},
			validate: func(t *testing.T, got Trace) {
				if got.TraceStart.IsZero() {
					t.Error("TraceStart should default to now, got zero time")
				}
				if got.TraceStart.After(time.Now().UTC().Add(time.Second)) {
					t.Errorf("TraceStart unexpectedly in the future: %v", got.TraceStart)
				}
			},
		},
		{
			name:  "invalid uuid is regenerated",
			class: "corner/security",
			header: http.Header{
				"X-Trace-Id":   {"'; DROP TABLE traces;--"},
				"X-Request-Id": {strings.Repeat("z", 10_000)},
			},
			validate: func(t *testing.T, got Trace) {
				mustUUID(t, "TraceID", got.TraceID)
				mustUUID(t, "RequestID", got.RequestID)
				if strings.Contains(got.TraceID, "DROP") {
					t.Error("hostile TraceID echoed back verbatim")
				}
				if len(got.RequestID) > 64 {
					t.Errorf("oversized RequestID not rejected: len=%d", len(got.RequestID))
				}
			},
		},
		{
			name:  "control chars in source are stripped",
			class: "security",
			header: http.Header{
				"X-Trace-Source":   {"evil\r\nX-Injected: pwned"},
				"X-Request-Source": {strings.Repeat("a", 1000)},
			},
			validate: func(t *testing.T, got Trace) {
				if strings.ContainsAny(got.TraceSource, "\r\n") {
					t.Errorf("CRLF not stripped from TraceSource: %q", got.TraceSource)
				}
				for _, r := range got.TraceSource {
					if unicode.IsControl(r) {
						t.Errorf("control char survived sanitization: %q", got.TraceSource)
					}
				}
				if n := len([]rune(got.RequestSource)); n > maxSourceLen {
					t.Errorf("oversized source not capped: %d runes", n)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.class+"/"+c.name, func(t *testing.T) {
			got := FromHeaderOrNew(c.header)
			// RequestStart is always "now" regardless of input.
			if got.RequestStart.IsZero() {
				t.Error("RequestStart should always be set")
			}
			c.validate(t, got)
		})
	}
}

func TestValidUUIDOrNew(t *testing.T) {
	valid := uuid.NewString()
	cases := []struct {
		name, class, in string
		wantPassthrough bool
	}{
		{"valid uuid passes through", "positive", valid, true},
		{"empty regenerated", "negative", "", false},
		{"garbage regenerated", "corner", "not-a-uuid", false},
		{"sql injection regenerated", "security", "1' OR '1'='1", false},
		{"huge input regenerated", "boundary", strings.Repeat("f", 100_000), false},
	}
	for _, c := range cases {
		t.Run(c.class+"/"+c.name, func(t *testing.T) {
			got := validUUIDOrNew(c.in)
			mustUUID(t, "result", got)
			if c.wantPassthrough && got != c.in {
				t.Errorf("valid input should pass through: got %q want %q", got, c.in)
			}
			if !c.wantPassthrough && got == c.in {
				t.Errorf("invalid input should have been regenerated, got %q", got)
			}
		})
	}
}

func TestSanitizeSource(t *testing.T) {
	cases := []struct {
		name, class, in, want string
	}{
		{"plain passes through", "positive", "api-gateway", "api-gateway"},
		{"empty stays empty", "negative", "", ""},
		{"crlf stripped", "security", "a\r\nb", "ab"},
		{"tabs and nulls stripped", "security", "a\tb\x00c", "abc"},
		{"exactly max length kept", "boundary", strings.Repeat("x", maxSourceLen), strings.Repeat("x", maxSourceLen)},
		{"over max length truncated", "boundary", strings.Repeat("x", maxSourceLen+50), strings.Repeat("x", maxSourceLen)},
	}
	for _, c := range cases {
		t.Run(c.class+"/"+c.name, func(t *testing.T) {
			if got := sanitizeSource(c.in); got != c.want {
				t.Errorf("sanitizeSource(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestClientMiddleware(t *testing.T) {
	// captured holds the request the inner RoundTripper actually sees.
	var captured *http.Request
	inner := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		captured = r
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})
	rt := ClientMiddleware(inner)

	t.Run("positive/no existing trace mints one and sets headers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com", nil)
		if _, err := rt.RoundTrip(req); err != nil {
			t.Fatal(err)
		}
		mustUUID(t, "X-Trace-ID", captured.Header.Get("X-Trace-ID"))
		mustUUID(t, "X-Request-ID", captured.Header.Get("X-Request-ID"))
		if _, ok := FromCtx(captured.Context()); !ok {
			t.Error("trace should be injected into the outgoing request context")
		}
	})

	t.Run("positive/existing trace reused with fresh request id", func(t *testing.T) {
		parent := New()
		req := httptest.NewRequest("GET", "http://example.com", nil)
		req = req.WithContext(CtxWith(req.Context(), parent))
		if _, err := rt.RoundTrip(req); err != nil {
			t.Fatal(err)
		}
		if got := captured.Header.Get("X-Trace-ID"); got != parent.TraceID {
			t.Errorf("TraceID should be reused: got %q want %q", got, parent.TraceID)
		}
		if got := captured.Header.Get("X-Request-ID"); got == parent.RequestID {
			t.Error("a sub-request should get a fresh RequestID")
		} else {
			mustUUID(t, "sub-request X-Request-ID", got)
		}
	})
}

func TestServerMiddleware(t *testing.T) {
	var seen Trace
	var ok bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, ok = FromCtx(r.Context())
		w.WriteHeader(200)
	})
	h := ServerMiddleware(next)

	t.Run("positive/inbound trace headers are propagated to context", func(t *testing.T) {
		id := uuid.NewString()
		req := httptest.NewRequest("GET", "http://example.com", nil)
		req.Header.Set("X-Trace-ID", id)
		h.ServeHTTP(httptest.NewRecorder(), req)
		if !ok {
			t.Fatal("handler should see a trace in context")
		}
		if seen.TraceID != id {
			t.Errorf("inbound TraceID not propagated: got %q want %q", seen.TraceID, id)
		}
	})

	t.Run("negative/no headers still yields a valid trace", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com", nil)
		h.ServeHTTP(httptest.NewRecorder(), req)
		if !ok {
			t.Fatal("handler should see a generated trace")
		}
		mustUUID(t, "generated TraceID", seen.TraceID)
	})
}

// --- Race / concurrency tests (run with -race) ---

func TestConcurrentNewAndHeader(t *testing.T) {
	const goroutines = 50
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for range 200 {
				tr := New()
				h := http.Header{}
				SaveToHeader(h, tr)
				got := FromHeaderOrNew(h)
				if got.TraceID != tr.TraceID {
					t.Errorf("concurrent round-trip mismatch: %q != %q", got.TraceID, tr.TraceID)
				}
			}
		})
	}
	wg.Wait()
}

func TestConcurrentMiddleware(t *testing.T) {
	rt := ClientMiddleware(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}))
	srv := ServerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = FromCtx(r.Context())
	}))

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			for range 100 {
				req := httptest.NewRequest("GET", "http://example.com", nil)
				_, _ = rt.RoundTrip(req)
				srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "http://example.com", nil))
			}
		})
	}
	wg.Wait()
}

// --- Benchmarks ---

func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = New()
	}
}

func BenchmarkNewUUID(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = newuuid()
	}
}

func BenchmarkFromHeaderOrNew(b *testing.B) {
	valid := http.Header{
		"X-Trace-Id":     {uuid.NewString()},
		"X-Request-Id":   {uuid.NewString()},
		"X-Trace-Start":  {time.Now().UTC().Format(time.RFC3339)},
		"X-Trace-Source": {"gateway"},
	}
	invalid := http.Header{
		"X-Trace-Id":     {"garbage"},
		"X-Request-Id":   {"garbage"},
		"X-Trace-Source": {"evil\r\ninjection"},
	}
	empty := http.Header{}

	b.Run("valid", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = FromHeaderOrNew(valid)
		}
	})
	b.Run("invalid", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = FromHeaderOrNew(invalid)
		}
	})
	b.Run("missing", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = FromHeaderOrNew(empty)
		}
	})
}

func BenchmarkSaveToHeader(b *testing.B) {
	tr := New()
	h := http.Header{}
	b.ReportAllocs()
	for b.Loop() {
		SaveToHeader(h, tr)
	}
}
