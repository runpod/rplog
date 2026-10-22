// Package tracegin provides trace propagation middleware for gin.
package tracegin

import (
	"github.com/gin-gonic/gin"
	"github.com/runpod/rplog/trace"
)

// Middleware is the gin equivalent of trace.ServerMiddleware.
// It must be the first middleware registered so later middleware and handlers see the Trace.
func Middleware(c *gin.Context) {
	t := trace.FromHeaderOrNew(c.Request.Header)
	c.Request = c.Request.WithContext(trace.CtxWith(c.Request.Context(), t))
	c.Next()
}
