package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// TimeoutMiddleware sets a deadline on the request context. When the deadline
// passes, any downstream call that respects context (Cassandra, Redis, HTTP)
// will automatically return an error, causing the handler to respond naturally.
//
// This replaces the previous goroutine-based approach which had a data race:
// c.Next() was called inside a goroutine while Gin's ResponseWriter is not
// goroutine-safe (both the timeout branch and handler branch could write).
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
