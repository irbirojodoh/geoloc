package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// TimeoutMiddleware wraps the request context with a timeout to prevent slow slow clients or external
// APIs (like Nominatim geocoding) from exhausting the server's goroutine pool.
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Create a context with a timeout
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		// Replace the request's context
		c.Request = c.Request.WithContext(ctx)

		// Create a channel to know when the custom handler finishes
		done := make(chan struct{})

		go func() {
			c.Next()
			close(done)
		}()

		// Wait for either the handler to finish or the timeout
		select {
		case <-done:
			return
		case <-ctx.Done():
			// Handle the timeout
			c.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{
				"error": "Request timed out",
			})
		}
	}
}
