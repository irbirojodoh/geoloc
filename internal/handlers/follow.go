package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gocql/gocql"
	"time"
	"context"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
	"social-geo-go/internal/notifications"
	"social-geo-go/internal/notifications/kafka"
)

// FollowUser handles POST /api/v1/users/:id/follow
func FollowUser(followRepo *data.FollowRepository, notifDispatcher *notifications.NotificationDispatcher) gin.HandlerFunc {
	return func(c *gin.Context) {
		followerID := auth.GetUserID(c)
		followingID := c.Param("id")

		if followerID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		err := followRepo.Follow(c.Request.Context(), followerID, followingID)
		if err != nil {
			status := http.StatusInternalServerError
			if err.Error() == "cannot follow yourself" {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Create notification for followed user
		if notifDispatcher != nil {
			go notifDispatcher.Dispatch(context.Background(), &kafka.NotificationEvent{
				EventID:     gocql.TimeUUID().String(),
				EventType:   data.NotificationTypeFollow,
				ActorID:     followerID,
				RecipientID: followingID,
				TargetType:  "user",
				TargetID:    followerID,
				Message:     "started following you",
				CreatedAt:   time.Now().Format(time.RFC3339),
			})
		}

		c.JSON(http.StatusOK, gin.H{"message": "User followed"})
	}
}

// UnfollowUser handles DELETE /api/v1/users/:id/follow
func UnfollowUser(followRepo *data.FollowRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		followerID := auth.GetUserID(c)
		followingID := c.Param("id")

		if followerID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		err := followRepo.Unfollow(c.Request.Context(), followerID, followingID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "User unfollowed"})
	}
}

// GetFollowers handles GET /api/v1/users/:id/followers
func GetFollowers(followRepo *data.FollowRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("id")

		limit := 50
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil {
				limit = parsed
			}
		}

		followers, err := followRepo.GetFollowers(c.Request.Context(), userID, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		counts, _ := followRepo.GetFollowCounts(c.Request.Context(), userID)

		c.JSON(http.StatusOK, gin.H{
			"user_id":   userID,
			"count":     counts.FollowersCount,
			"followers": followers,
		})
	}
}

// GetFollowing handles GET /api/v1/users/:id/following
func GetFollowing(followRepo *data.FollowRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("id")

		limit := 50
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil {
				limit = parsed
			}
		}

		following, err := followRepo.GetFollowing(c.Request.Context(), userID, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		counts, _ := followRepo.GetFollowCounts(c.Request.Context(), userID)

		c.JSON(http.StatusOK, gin.H{
			"user_id":   userID,
			"count":     counts.FollowingCount,
			"following": following,
		})
	}
}
