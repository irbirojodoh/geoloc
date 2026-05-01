package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
)

// DeleteAccountRequest represents the request body for account deletion
type DeleteAccountRequest struct {
	Password string `json:"password" binding:"required"`
}

// DeleteAccount handles DELETE /api/v1/users/me
// Requires password confirmation to prevent CSRF-style deletion
func DeleteAccount(userRepo *data.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		var req DeleteAccountRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Password confirmation is required to delete your account",
			})
			return
		}

		// Fetch user to verify password
		user, err := userRepo.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			slog.Error("Failed to fetch user for deletion", "error", err, "user_id", userID)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to process account deletion",
			})
			return
		}

		// OAuth users have empty password hash — they must use a different flow
		if user.PasswordHash == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "OAuth accounts cannot be deleted via password. Please contact support.",
			})
			return
		}

		// Verify password
		if !auth.VerifyPassword(req.Password, user.PasswordHash) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Incorrect password",
			})
			return
		}

		// Perform soft delete
		err = userRepo.SoftDeleteUser(c.Request.Context(), userID)
		if err != nil {
			slog.Error("Failed to soft-delete user", "error", err, "user_id", userID)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to delete account",
			})
			return
		}

		slog.Info("[ACCOUNT] User account deleted", "user_id", userID)

		c.JSON(http.StatusOK, gin.H{
			"message": "Your account has been deleted. All personal data has been anonymized.",
		})
	}
}
