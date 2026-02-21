package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
)

// ToggleLikeRequest represents a toggle like request body
type ToggleLikeRequest struct {
	Like bool `json:"like"` // true = like, false = unlike
}

// TogglePostLike handles POST /api/v1/posts/:id/toggle-like
// This is the idempotent version - safe for retries and double-clicks
func TogglePostLike(likeRepo *data.LikeRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		postID := c.Param("id")
		userID := auth.GetUserID(c)

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		var req ToggleLikeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			// Default to like if no body provided
			req.Like = true
		}

		result, err := likeRepo.ToggleLike(c.Request.Context(), data.TargetTypePost, postID, userID, req.Like)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to toggle like",
			})
			return
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
func ToggleCommentLike(likeRepo *data.LikeRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		commentID := c.Param("id")
		userID := auth.GetUserID(c)

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		var req ToggleLikeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			req.Like = true
		}

		result, err := likeRepo.ToggleLike(c.Request.Context(), data.TargetTypeComment, commentID, userID, req.Like)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to toggle like",
			})
			return
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
