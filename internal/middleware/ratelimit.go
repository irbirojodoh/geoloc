package middleware

import (
	"context"
	"net/http"
	"time"

	"social-geo-go/internal/cache"

	"github.com/gin-gonic/gin"
)

// RateLimiter uses Redis to track limits across multiple instances
type RateLimiter struct {
	redis  *cache.RedisClient
	limit  int
	window time.Duration
}

// NewRateLimiter creates a new Redis-backed rate limiter
func NewRateLimiter(redis *cache.RedisClient, limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		redis:  redis,
		limit:  limit,
		window: window,
	}
}

// Allow checks if a request is allowed using Redis INCR and EXPIRE
func (rl *RateLimiter) Allow(ctx context.Context, key string) bool {
	if rl.redis == nil {
		// Fallback if Redis is down: allow request but log warning ideally
		return true
	}

	redisKey := "ratelimit:" + key

	// Increment the counter
	count, err := rl.redis.Client().Incr(ctx, redisKey).Result()
	if err != nil {
		return true // Fail open on Redis errors
	}

	// If it's the first request in the window, set the expiration
	if count == 1 {
		rl.redis.Client().Expire(ctx, redisKey, rl.window)
	}

	return count <= int64(rl.limit)
}

// RateLimitByIP creates middleware that rate limits by IP
func RateLimitByIP(redis *cache.RedisClient, limit int, window time.Duration) gin.HandlerFunc {
	limiter := NewRateLimiter(redis, limit, window)

	return func(c *gin.Context) {
		ip := c.ClientIP()

		if !limiter.Allow(c.Request.Context(), "ip:"+ip) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded",
				"retry_after": window.Seconds(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RateLimitByUser creates middleware that rate limits by user ID
func RateLimitByUser(redis *cache.RedisClient, limit int, window time.Duration) gin.HandlerFunc {
	limiter := NewRateLimiter(redis, limit, window)

	return func(c *gin.Context) {
		// Get user ID from context (set by auth middleware)
		userID, exists := c.Get("user_id")
		key := "ip:" + c.ClientIP()
		if exists {
			key = "user:" + userID.(string)
		}

		if !limiter.Allow(c.Request.Context(), key) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded",
				"retry_after": window.Seconds(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
