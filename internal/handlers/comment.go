package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
)

// CreateComment handles POST /api/v1/posts/:id/comments
func CreateComment(commentRepo *data.CommentRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		postID := c.Param("id")
		userID := auth.GetUserID(c)

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		var req data.CreateCommentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request body",
			})
			return
		}

		req.PostID = postID
		req.UserID = userID
		req.IPAddress = c.ClientIP()
		req.UserAgent = c.Request.UserAgent()

		comment, err := commentRepo.CreateComment(c.Request.Context(), &req)
		if err != nil {
			status := http.StatusInternalServerError
			if err.Error() == "parent comment not found" ||
				err.Error() == "maximum comment depth of 3 reached" {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{
				"error":   "Failed to create comment",
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Comment created",
			"comment": comment,
		})
	}
}

// GetComments handles GET /api/v1/posts/:id/comments
func GetComments(commentRepo *data.CommentRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		postID := c.Param("id")

		limit := 50
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil {
				limit = parsed
			}
		}

		comments, err := commentRepo.GetCommentsForPost(c.Request.Context(), postID, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to get comments",
			})
			return
		}

		count, _ := commentRepo.GetCommentCount(c.Request.Context(), postID)

		c.JSON(http.StatusOK, gin.H{
			"post_id":     postID,
			"total_count": count,
			"comments":    comments,
		})
	}
}

// ReplyToComment handles POST /api/v1/comments/:id/reply
func ReplyToComment(commentRepo *data.CommentRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		parentID := c.Param("id")
		userID := auth.GetUserID(c)

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		// Get parent comment to find post_id
		parent, err := commentRepo.GetCommentByID(c.Request.Context(), parentID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Parent comment not found"})
			return
		}

		var req data.CreateCommentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request body",
			})
			return
		}

		req.PostID = parent.PostID
		req.ParentID = parentID
		req.UserID = userID
		req.IPAddress = c.ClientIP()
		req.UserAgent = c.Request.UserAgent()

		comment, err := commentRepo.CreateComment(c.Request.Context(), &req)
		if err != nil {
			status := http.StatusInternalServerError
			if err.Error() == "maximum comment depth of 3 reached" {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{
				"error":   "Failed to create reply",
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Reply created",
			"comment": comment,
		})
	}
}

// DeleteComment handles DELETE /api/v1/comments/:id
func DeleteComment(commentRepo *data.CommentRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		commentID := c.Param("id")
		userID := auth.GetUserID(c)

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		err := commentRepo.DeleteComment(c.Request.Context(), commentID, userID)
		if err != nil {
			status := http.StatusInternalServerError
			if err.Error() == "comment not found" {
				status = http.StatusNotFound
			} else if err.Error() == "unauthorized: can only delete your own comments" {
				status = http.StatusForbidden
			}
			c.JSON(status, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Comment deleted"})
	}
}
