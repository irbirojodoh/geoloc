package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
)

// UpdateProfile handles PUT /api/v1/users/me
func UpdateProfile(userRepo *data.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		var req data.UpdateProfileRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request body",
			})
			return
		}

		user, err := userRepo.UpdateUser(c.Request.Context(), userID, req.FullName, req.Bio, req.ProfilePictureURL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to update profile",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Profile updated",
			"user":    user,
		})
	}
}
