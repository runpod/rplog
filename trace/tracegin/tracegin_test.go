package tracegin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/runpod/rplog/trace"
)

func init() { gin.SetMode(gin.TestMode) }

func TestMiddlewareCreatesTraceWithoutHeaders(t *testing.T) {
	var seen trace.Trace
	var ok bool
	r := gin.New()
	r.Use(Middleware)
	r.GET("/", func(c *gin.Context) { seen, ok = trace.FromCtx(c.Request.Context()) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !ok {
		t.Fatal("expected a Trace in the request context")
	}
	if seen.TraceID == "" {
		t.Error("TraceID is empty")
	}
}

func TestMiddlewarePreservesIncomingTraceID(t *testing.T) {
	var seen trace.Trace
	var ok bool
	r := gin.New()
	r.Use(Middleware)
	r.GET("/", func(c *gin.Context) { seen, ok = trace.FromCtx(c.Request.Context()) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	trace.SaveToHeader(req.Header, trace.Trace{
		TraceID:    "trace-from-header",
		RequestID:  "request-from-header",
		TraceStart: time.Now().Add(-time.Minute),
	})
	r.ServeHTTP(httptest.NewRecorder(), req)
	if !ok {
		t.Fatal("expected a Trace in the request context")
	}
	if seen.TraceID != "trace-from-header" {
		t.Errorf("TraceID = %q, want %q", seen.TraceID, "trace-from-header")
	}
}

// Inbound trace headers must survive Middleware -> c.Request.Context() ->
// trace.ClientMiddleware and arrive on the downstream request.
func TestHeadersRoundTripOverTheWire(t *testing.T) {
	const (
		traceID   = "5f9c2e6a-1b3d-4c8e-9a0f-2b7c6d5e4f31"
		requestID = "018f3a1c-2b4d-7e8f-9a0b-1c2d3e4f5061"
		source    = "runpod-graphql"
	)
	var downstream http.Header
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downstream = r.Header.Clone()
	}))
	defer down.Close()

	client := &http.Client{Transport: trace.ClientMiddleware(http.DefaultTransport)}
	r := gin.New()
	r.Use(Middleware)
	r.GET("/", func(c *gin.Context) {
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, down.URL, nil)
		if err != nil {
			t.Errorf("NewRequest: %v", err)
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Errorf("downstream request: %v", err)
			return
		}
		resp.Body.Close()
	})
	mid := httptest.NewServer(r)
	defer mid.Close()

	start := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	req, _ := http.NewRequest(http.MethodGet, mid.URL, nil)
	req.Header.Set("X-Trace-ID", traceID)
	req.Header.Set("X-Request-ID", requestID)
	req.Header.Set("X-Trace-Source", source)
	req.Header.Set("X-Trace-Start", start.Format(time.RFC3339))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("inbound request: %v", err)
	}
	resp.Body.Close()

	if downstream == nil {
		t.Fatal("downstream server received no request")
	}
	if got := downstream.Get("X-Trace-ID"); got != traceID {
		t.Errorf("X-Trace-ID = %q, want %q", got, traceID)
	}
	if got := downstream.Get("X-Request-ID"); got == "" || got == requestID {
		t.Errorf("X-Request-ID = %q, want a fresh id", got)
	}
	if got := downstream.Get("X-Trace-Source"); got != source {
		t.Errorf("X-Trace-Source = %q, want %q", got, source)
	}
	if got := downstream.Get("X-Trace-Start"); got != start.Format(time.RFC3339) {
		t.Errorf("X-Trace-Start = %q, want %q", got, start.Format(time.RFC3339))
	}
}

func TestMiddlewareVisibleToLaterMiddleware(t *testing.T) {
	var ok bool
	r := gin.New()
	r.Use(Middleware, func(c *gin.Context) {
		_, ok = trace.FromCtx(c.Request.Context())
		c.Next()
	})
	r.GET("/", func(c *gin.Context) {})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !ok {
		t.Error("later middleware did not see a Trace in the request context")
	}
}
