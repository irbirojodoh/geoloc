package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ssePaths are endpoints that must stay open indefinitely (SSE streams).
var ssePaths = []string{
	"/api/v1/notifications/stream",
}

// TimeoutMiddleware sets a deadline on the request context. When the deadline
// passes, any downstream call that respects context (Cassandra, Redis, HTTP)
// will automatically return an error, causing the handler to respond naturally.
//
// This replaces the previous goroutine-based approach which had a data race:
// c.Next() was called inside a goroutine while Gin's ResponseWriter is not
// goroutine-safe (both the timeout branch and handler branch could write).
//
// SSE endpoints are excluded from the timeout since they are long-lived.
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip timeout for long-lived SSE connections
		for _, path := range ssePaths {
			if strings.EqualFold(c.Request.URL.Path, path) {
				c.Next()
				return
			}
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
