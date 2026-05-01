package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
)

// ============== REPORTS ==============

// CreateReportRequest represents the request body for reporting content
type CreateReportRequest struct {
	TargetType  string `json:"target_type" binding:"required"`  // "post", "comment", "user"
	TargetID    string `json:"target_id" binding:"required"`
	Reason      string `json:"reason" binding:"required"`       // "spam", "harassment", "inappropriate", "other"
	Description string `json:"description"`                     // Optional details
}

// CreateReport handles POST /api/v1/reports
func CreateReport(modRepo *data.ModerationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		var req CreateReportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request body. target_type, target_id, and reason are required.",
			})
			return
		}

		// Validate target type
		if !data.ValidTargetTypes[req.TargetType] {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid target_type. Must be 'post', 'comment', or 'user'.",
			})
			return
		}

		// Validate reason
		if !data.ValidReportReasons[req.Reason] {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid reason. Must be 'spam', 'harassment', 'inappropriate', or 'other'.",
			})
			return
		}

		// Cannot report yourself
		if req.TargetType == "user" && req.TargetID == userID {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "You cannot report yourself",
			})
			return
		}

		// Check for duplicate report
		alreadyReported, err := modRepo.HasReported(c.Request.Context(), userID, req.TargetType, req.TargetID)
		if err != nil {
			slog.Error("Failed to check duplicate report", "error", err)
		}
		if alreadyReported {
			c.JSON(http.StatusConflict, gin.H{
				"error": "You have already reported this content",
			})
			return
		}

		// Limit description length
		if len(req.Description) > 1000 {
			req.Description = req.Description[:1000]
		}

		err = modRepo.CreateReport(c.Request.Context(), userID, req.TargetType, req.TargetID, req.Reason, req.Description)
		if err != nil {
			slog.Error("Failed to create report", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to submit report",
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Report submitted successfully. Our team will review it.",
		})
	}
}

// ============== BLOCKS ==============

// BlockUser handles POST /api/v1/users/:id/block
func BlockUser(modRepo *data.ModerationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		targetID := c.Param("id")

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		if userID == targetID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "You cannot block yourself"})
			return
		}

		err := modRepo.BlockUser(c.Request.Context(), userID, targetID)
		if err != nil {
			slog.Error("Failed to block user", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to block user",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "User blocked"})
	}
}

// UnblockUser handles DELETE /api/v1/users/:id/block
func UnblockUser(modRepo *data.ModerationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		targetID := c.Param("id")

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		err := modRepo.UnblockUser(c.Request.Context(), userID, targetID)
		if err != nil {
			slog.Error("Failed to unblock user", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to unblock user",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "User unblocked"})
	}
}

// GetBlockedUsers handles GET /api/v1/users/me/blocked
func GetBlockedUsers(modRepo *data.ModerationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		blocked, err := modRepo.GetBlockedUsers(c.Request.Context(), userID)
		if err != nil {
			slog.Error("Failed to get blocked users", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to get blocked users",
			})
			return
		}

		if blocked == nil {
			blocked = []string{}
		}

		c.JSON(http.StatusOK, gin.H{
			"blocked_users": blocked,
			"count":         len(blocked),
		})
	}
}

// ============== MUTES ==============

// MuteUser handles POST /api/v1/users/:id/mute
func MuteUser(modRepo *data.ModerationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		targetID := c.Param("id")

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		if userID == targetID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "You cannot mute yourself"})
			return
		}

		err := modRepo.MuteUser(c.Request.Context(), userID, targetID)
		if err != nil {
			slog.Error("Failed to mute user", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to mute user",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "User muted"})
	}
}

// UnmuteUser handles DELETE /api/v1/users/:id/mute
func UnmuteUser(modRepo *data.ModerationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		targetID := c.Param("id")

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		err := modRepo.UnmuteUser(c.Request.Context(), userID, targetID)
		if err != nil {
			slog.Error("Failed to unmute user", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to unmute user",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "User unmuted"})
	}
}

// GetMutedUsers handles GET /api/v1/users/me/muted
func GetMutedUsers(modRepo *data.ModerationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		muted, err := modRepo.GetMutedUsers(c.Request.Context(), userID)
		if err != nil {
			slog.Error("Failed to get muted users", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to get muted users",
			})
			return
		}

		if muted == nil {
			muted = []string{}
		}

		c.JSON(http.StatusOK, gin.H{
			"muted_users": muted,
			"count":       len(muted),
		})
	}
}
