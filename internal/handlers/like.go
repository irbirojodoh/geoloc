package handlers

import (
	"context"
	"net/http"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/gocql/gocql"
	"time"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
	"social-geo-go/internal/notifications"
	"social-geo-go/internal/notifications/kafka"
)

func truncateText(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// ToggleLikeRequest represents a toggle like request body.
// If "like" is omitted, the current state is flipped (true toggle).
type ToggleLikeRequest struct {
	Like *bool `json:"like"` // true = like, false = unlike, omitted = toggle
}

func resolveWantLiked(ctx context.Context, likeRepo *data.LikeRepository, targetType, targetID, userID string, explicit *bool) (bool, error) {
	if explicit != nil {
		return *explicit, nil
	}
	liked, err := likeRepo.HasUserLiked(ctx, targetType, targetID, userID)
	if err != nil {
		return false, err
	}
	return !liked, nil
}

// TogglePostLike handles POST /api/v1/posts/:id/toggle-like
// This is the idempotent version - safe for retries and double-clicks
func TogglePostLike(likeRepo *data.LikeRepository, postRepo *data.PostRepository, notifDispatcher *notifications.NotificationDispatcher) gin.HandlerFunc {
	return func(c *gin.Context) {
		postID := c.Param("id")
		userID := auth.GetUserID(c)

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		var req ToggleLikeRequest
		_ = c.ShouldBindJSON(&req)

		wantLiked, err := resolveWantLiked(c.Request.Context(), likeRepo, data.TargetTypePost, postID, userID, req.Like)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read like state"})
			return
		}

		result, err := likeRepo.ToggleLike(c.Request.Context(), data.TargetTypePost, postID, userID, wantLiked)
		if err != nil {
			slog.Error("TogglePostLike failed", "error", err, "post_id", postID, "user_id", userID, "want_liked", wantLiked)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to toggle like",
			})
			return
		}

		if result.Changed && result.IsLiked && notifDispatcher != nil {
			post, _ := postRepo.GetPostByID(c.Request.Context(), postID)
			if post != nil && post.UserID != userID {
				go notifDispatcher.Dispatch(context.Background(), &kafka.NotificationEvent{
					EventID:     gocql.TimeUUID().String(),
					EventType:   data.NotificationTypeLike,
					ActorID:     userID,
					RecipientID: post.UserID,
					TargetType:  data.TargetTypePost,
					TargetID:    postID,
					Message:     "liked your post",
					Payload:     map[string]string{"post_preview": truncateText(post.Content, 100)},
					CreatedAt:   time.Now().Format(time.RFC3339),
				})
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"is_liked":   result.IsLiked,
			"like_count": result.LikeCount,
			"changed":    result.Changed,
		})
	}
}

// ToggleCommentLike handles POST /api/v1/comments/:id/toggle-like
// This is the idempotent version - safe for retries and double-clicks
func ToggleCommentLike(likeRepo *data.LikeRepository, commentRepo *data.CommentRepository, notifDispatcher *notifications.NotificationDispatcher) gin.HandlerFunc {
	return func(c *gin.Context) {
		commentID := c.Param("id")
		userID := auth.GetUserID(c)

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		var req ToggleLikeRequest
		_ = c.ShouldBindJSON(&req)

		wantLiked, err := resolveWantLiked(c.Request.Context(), likeRepo, data.TargetTypeComment, commentID, userID, req.Like)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read like state"})
			return
		}

		result, err := likeRepo.ToggleLike(c.Request.Context(), data.TargetTypeComment, commentID, userID, wantLiked)
		if err != nil {
			slog.Error("ToggleCommentLike failed", "error", err, "comment_id", commentID, "user_id", userID, "want_liked", wantLiked)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to toggle like",
			})
			return
		}

		if result.Changed && result.IsLiked && notifDispatcher != nil {
			comment, _ := commentRepo.GetCommentByID(c.Request.Context(), commentID)
			if comment != nil && comment.UserID != userID {
				go notifDispatcher.Dispatch(context.Background(), &kafka.NotificationEvent{
					EventID:     gocql.TimeUUID().String(),
					EventType:   data.NotificationTypeLike,
					ActorID:     userID,
					RecipientID: comment.UserID,
					TargetType:  data.TargetTypeComment,
					TargetID:    commentID,
					Message:     "liked your comment",
					Payload:     map[string]string{"comment_preview": truncateText(comment.Content, 100)},
					CreatedAt:   time.Now().Format(time.RFC3339),
				})
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"is_liked":   result.IsLiked,
			"like_count": result.LikeCount,
			"changed":    result.Changed,
		})
	}
}

// ============== LEGACY ENDPOINTS (backward compatible) ==============

// LikePost handles POST /api/v1/posts/:id/like
func LikePost(likeRepo *data.LikeRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Deprecation", "true")
		c.Header("Sunset", "2026-08-01")
		c.Header("Link", "</api/v1/posts/{id}/toggle-like>; rel=\"successor-version\"")
		
		postID := c.Param("id")
		userID := auth.GetUserID(c)

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		result, err := likeRepo.ToggleLike(c.Request.Context(), data.TargetTypePost, postID, userID, true)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to like post",
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message":    "Post liked",
			"is_liked":   result.IsLiked,
			"like_count": result.LikeCount,
		})
	}
}

// UnlikePost handles DELETE /api/v1/posts/:id/like
func UnlikePost(likeRepo *data.LikeRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Deprecation", "true")
		c.Header("Sunset", "2026-08-01")
		c.Header("Link", "</api/v1/posts/{id}/toggle-like>; rel=\"successor-version\"")

		postID := c.Param("id")
		userID := auth.GetUserID(c)

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		result, err := likeRepo.ToggleLike(c.Request.Context(), data.TargetTypePost, postID, userID, false)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to unlike post",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":    "Post unliked",
			"is_liked":   result.IsLiked,
			"like_count": result.LikeCount,
		})
	}
}

// LikeComment handles POST /api/v1/comments/:id/like
func LikeComment(likeRepo *data.LikeRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Deprecation", "true")
		c.Header("Sunset", "2026-08-01")
		c.Header("Link", "</api/v1/comments/{id}/toggle-like>; rel=\"successor-version\"")

		commentID := c.Param("id")
		userID := auth.GetUserID(c)

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		result, err := likeRepo.ToggleLike(c.Request.Context(), data.TargetTypeComment, commentID, userID, true)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to like comment",
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message":    "Comment liked",
			"is_liked":   result.IsLiked,
			"like_count": result.LikeCount,
		})
	}
}

// UnlikeComment handles DELETE /api/v1/comments/:id/like
func UnlikeComment(likeRepo *data.LikeRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Deprecation", "true")
		c.Header("Sunset", "2026-08-01")
		c.Header("Link", "</api/v1/comments/{id}/toggle-like>; rel=\"successor-version\"")

		commentID := c.Param("id")
		userID := auth.GetUserID(c)

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		result, err := likeRepo.ToggleLike(c.Request.Context(), data.TargetTypeComment, commentID, userID, false)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to unlike comment",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":    "Comment unliked",
			"is_liked":   result.IsLiked,
			"like_count": result.LikeCount,
		})
	}
}

// GetLikedPosts handles GET /api/v1/users/:id/liked-posts
func GetLikedPosts(likeRepo *data.LikeRepository, postRepo *data.PostRepository, userRepo *data.UserRepository, locRepo *data.LocationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("id")

		c.JSON(http.StatusOK, gin.H{
			"user_id":     userID,
			"count":       0,
			"data":        []data.Post{},
			"has_more":    false,
			"next_cursor": "",
			"message":     "Reading from likes_by_user not fully implemented yet",
		})
	}
}
