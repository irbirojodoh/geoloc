package sse

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"social-geo-go/internal/auth"
)

// StreamNotifications handles GET /api/v1/notifications/stream
func StreamNotifications(redisClient *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == "" {
			c.AbortWithStatusJSON(401, gin.H{"error": "Unauthorized"})
			return
		}

		// Setup SSE headers
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		// Important: If reverse proxying through Caddy/Nginx, ensure buffering is off
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Writer.Flush()

		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()

		channel := fmt.Sprintf("sse:user:%s", userID)
		pubsub := redisClient.Subscribe(ctx, channel)
		defer pubsub.Close()

		slog.Info("Client connected to SSE", "user_id", userID)

		// Create a ticker for heartbeats/keepalives (30s)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		// Send initial connected event
		c.SSEvent("connected", map[string]interface{}{
			"status": "connected",
			"time":   time.Now().Format(time.RFC3339),
		})
		c.Writer.Flush()

		c.Stream(func(w io.Writer) bool {
			select {
			case <-ctx.Done():
				return false
			case <-ticker.C:
				// Send heartbeat to keep connection alive
				c.SSEvent("ping", map[string]interface{}{
					"time": time.Now().Format(time.RFC3339),
				})
				c.Writer.Flush()
				return true
			case msg := <-pubsub.Channel():
				// Forward raw JSON string from Redis Pub/Sub as message data
				c.SSEvent("notification", msg.Payload)
				c.Writer.Flush()
				return true
			}
		})
		
		slog.Info("Client disconnected from SSE", "user_id", userID)
	}
}
