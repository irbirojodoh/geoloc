package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
)

// LikePost handles POST /api/v1/posts/:id/like
func LikePost(likeRepo *data.LikeRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		postID := c.Param("id")
		userID := auth.GetUserID(c)

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		req := &data.LikeRequest{
			TargetType: data.TargetTypePost,
			TargetID:   postID,
			UserID:     userID,
		}

		like, err := likeRepo.AddLike(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to like post",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Post liked",
			"like":    like,
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

		req := &data.LikeRequest{
			TargetType: data.TargetTypePost,
			TargetID:   postID,
			UserID:     userID,
		}

		err := likeRepo.RemoveLike(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to unlike post",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Post unliked"})
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

		req := &data.LikeRequest{
			TargetType: data.TargetTypeComment,
			TargetID:   commentID,
			UserID:     userID,
		}

		like, err := likeRepo.AddLike(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to like comment",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Comment liked",
			"like":    like,
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

		req := &data.LikeRequest{
			TargetType: data.TargetTypeComment,
			TargetID:   commentID,
			UserID:     userID,
		}

		err := likeRepo.RemoveLike(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to unlike comment",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Comment unliked"})
	}
}
