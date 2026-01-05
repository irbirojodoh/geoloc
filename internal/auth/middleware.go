package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	// AuthorizationHeader is the header key for the JWT token
	AuthorizationHeader = "Authorization"
	// BearerPrefix is the prefix for Bearer tokens
	BearerPrefix = "Bearer "
	// UserIDKey is the context key for the authenticated user ID
	UserIDKey = "user_id"
)

// AuthRequired is a middleware that validates JWT tokens
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader(AuthorizationHeader)
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization header required",
			})
			c.Abort()
			return
		}

		// Check for Bearer prefix
		if !strings.HasPrefix(authHeader, BearerPrefix) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid authorization header format. Use: Bearer <token>",
			})
			c.Abort()
			return
		}

		// Extract token
		tokenString := strings.TrimPrefix(authHeader, BearerPrefix)
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Token is required",
			})
			c.Abort()
			return
		}

		// Validate token
		claims, err := ValidateAccessToken(tokenString)
		if err != nil {
			status := http.StatusUnauthorized
			message := "Invalid token"

			switch err {
			case ErrExpiredToken:
				message = "Token has expired"
			case ErrWrongType:
				message = "Invalid token type"
			}

			c.JSON(status, gin.H{
				"error": message,
			})
			c.Abort()
			return
		}

		// Set user ID in context for downstream handlers
		c.Set(UserIDKey, claims.UserID)
		c.Next()
	}
}

// GetUserID retrieves the authenticated user ID from the context
func GetUserID(c *gin.Context) string {
	userID, exists := c.Get(UserIDKey)
	if !exists {
		return ""
	}
	return userID.(string)
}
