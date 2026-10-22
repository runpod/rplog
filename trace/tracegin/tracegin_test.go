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
