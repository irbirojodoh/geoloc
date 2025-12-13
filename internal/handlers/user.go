package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/data"
)

// CreateUser handles POST /api/v1/users
func CreateUser(repo *data.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req data.CreateUserRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request body",
				"details": err.Error(),
			})
			return
		}

		user, err := repo.CreateUser(c.Request.Context(), &req)
		if err != nil {
			// Check for unique constraint violations
			if contains(err.Error(), "duplicate key") || contains(err.Error(), "unique constraint") {
				c.JSON(http.StatusConflict, gin.H{
					"error": "Username or email already exists",
				})
				return
			}

			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to create user",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "User created successfully",
			"user":    user,
		})
	}
}

// GetUser handles GET /api/v1/users/:id
func GetUser(repo *data.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		
		// Validate UUID format (basic check)
		if len(id) != 36 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid user ID format",
			})
			return
		}

		user, err := repo.GetUserByID(c.Request.Context(), id)
		if err != nil {
			if contains(err.Error(), "no rows") {
				c.JSON(http.StatusNotFound, gin.H{
					"error": "User not found",
				})
				return
			}

			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to fetch user",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"user": user,
		})
	}
}

// GetUserByUsername handles GET /api/v1/users/username/:username
func GetUserByUsername(repo *data.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.Param("username")

		user, err := repo.GetUserByUsername(c.Request.Context(), username)
		if err != nil {
			if contains(err.Error(), "no rows") {
				c.JSON(http.StatusNotFound, gin.H{
					"error": "User not found",
				})
				return
			}

			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to fetch user",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"user": user,
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
