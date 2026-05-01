package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
)

// ForgotPasswordRequest represents the request body for password reset initiation
type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ResetPasswordRequest represents the request body for password reset completion
type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6,max=128"`
}

// ForgotPassword handles POST /auth/forgot-password
// Always returns 200 to prevent email enumeration attacks
func ForgotPassword(userRepo *data.UserRepository, resetRepo *data.PasswordResetRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ForgotPasswordRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request body",
			})
			return
		}

		// Always return same response regardless of whether email exists
		// This prevents email enumeration attacks
		successMsg := gin.H{
			"message": "If an account with that email exists, a password reset link has been sent.",
		}

		// Look up user by email
		user, err := userRepo.GetUserByEmail(c.Request.Context(), req.Email)
		if err != nil || user == nil {
			// User not found — return same success message to prevent enumeration
			c.JSON(http.StatusOK, successMsg)
			return
		}

		// Generate reset token
		token, err := resetRepo.CreateToken(c.Request.Context(), user.ID)
		if err != nil {
			slog.Error("Failed to create password reset token", "error", err, "user_id", user.ID)
			// Still return success to prevent enumeration
			c.JSON(http.StatusOK, successMsg)
			return
		}

		// MVP: Log the token to stdout (replace with email service in production)
		slog.Info("[PASSWORD RESET] Token generated",
			"email", req.Email,
			"token", token,
			"user_id", user.ID,
		)

		c.JSON(http.StatusOK, successMsg)
	}
}

// ResetPassword handles POST /auth/reset-password
func ResetPassword(userRepo *data.UserRepository, resetRepo *data.PasswordResetRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ResetPasswordRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request body. Token and new_password (min 6 chars) are required.",
			})
			return
		}

		// Validate the reset token
		userID, err := resetRepo.ValidateToken(c.Request.Context(), req.Token)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid or expired reset token",
			})
			return
		}

		// Hash the new password
		newHash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			slog.Error("Failed to hash new password", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to process password",
			})
			return
		}

		// Update the user's password
		err = userRepo.UpdatePassword(c.Request.Context(), userID, newHash)
		if err != nil {
			slog.Error("Failed to update password", "error", err, "user_id", userID)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to update password",
			})
			return
		}

		// Mark the token as used
		if err := resetRepo.MarkUsed(c.Request.Context(), req.Token); err != nil {
			slog.Error("Failed to mark reset token as used", "error", err)
			// Don't fail the request — password was already updated
		}

		slog.Info("[PASSWORD RESET] Password updated successfully", "user_id", userID)

		c.JSON(http.StatusOK, gin.H{
			"message": "Password has been reset successfully. Please log in with your new password.",
		})
	}
}
