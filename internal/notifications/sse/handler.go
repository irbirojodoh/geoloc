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
		if redisClient == nil {
			c.AbortWithStatusJSON(503, gin.H{"error": "realtime_unavailable"})
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

		notifCh := fmt.Sprintf("sse:user:%s", userID)
		dmCh := fmt.Sprintf("dm:%s", userID)
		pubsub := redisClient.Subscribe(ctx, notifCh, dmCh)
		defer pubsub.Close()

		slog.Info("Client connected to SSE", "user_id", userID)

		onlineKey := "sse:online:" + userID
		if err := redisClient.Set(ctx, onlineKey, "1", 90*time.Second).Err(); err != nil {
			slog.Warn("sse online presence set failed", "user_id", userID, "error", err)
		}
		defer func() {
			shCtx, shCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer shCancel()
			if err := redisClient.Del(shCtx, onlineKey).Err(); err != nil {
				slog.Warn("sse online presence delete failed", "user_id", userID, "error", err)
			}
		}()

		// Create a ticker for heartbeats/keepalives (30s)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		// Send initial connected event
		fmt.Fprintf(c.Writer, "event: connected\ndata: {\"status\":\"connected\",\"time\":\"%s\"}\n\n", time.Now().Format(time.RFC3339))
		c.Writer.Flush()

		c.Stream(func(w io.Writer) bool {
			select {
			case <-ctx.Done():
				return false
			case <-ticker.C:
				if err := redisClient.Set(ctx, onlineKey, "1", 90*time.Second).Err(); err != nil {
					slog.Warn("sse online presence refresh failed", "user_id", userID, "error", err)
				}
				// Send heartbeat comment to keep connection alive.
				fmt.Fprint(c.Writer, ": heartbeat\n\n")
				c.Writer.Flush()
				return true
			case msg := <-pubsub.Channel():
				if msg == nil {
					return false
				}
				// Forward raw JSON payload so clients can parse into AppNotification or DM events.
				fmt.Fprintf(c.Writer, "data: %s\n\n", msg.Payload)
				c.Writer.Flush()
				return true
			}
		})

		slog.Info("Client disconnected from SSE", "user_id", userID)
	}
}
