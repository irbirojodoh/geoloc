package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
)

// GetNotifications handles GET /api/v1/notifications
func GetNotifications(notifRepo *data.NotificationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		limit := 50
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil {
				limit = parsed
			}
		}

		unreadOnly := c.Query("unread") == "true"

		notifications, err := notifRepo.GetNotifications(c.Request.Context(), userID, limit, unreadOnly)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		unreadCount, _ := notifRepo.GetUnreadCount(c.Request.Context(), userID)

		c.JSON(http.StatusOK, gin.H{
			"notifications": notifications,
			"unread_count":  unreadCount,
			"total":         len(notifications),
		})
	}
}

// MarkNotificationAsRead handles PUT /api/v1/notifications/:id/read
func MarkNotificationAsRead(notifRepo *data.NotificationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		notificationID := c.Param("id")

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		err := notifRepo.MarkAsRead(c.Request.Context(), userID, notificationID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Notification marked as read"})
	}
}

// MarkAllNotificationsAsRead handles PUT /api/v1/notifications/read-all
func MarkAllNotificationsAsRead(notifRepo *data.NotificationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		err := notifRepo.MarkAllAsRead(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "All notifications marked as read"})
	}
}
