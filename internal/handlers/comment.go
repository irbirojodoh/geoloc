package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
)

// Helper to enrich comments with user info
func enrichCommentsWithUserInfo(ctx context.Context, comments []data.Comment, userRepo *data.UserRepository) {
	if userRepo == nil || len(comments) == 0 {
		return
	}
	userIDsMap := make(map[string]bool)
	var collectUserIDs func([]data.Comment)
	collectUserIDs = func(comments []data.Comment) {
		for _, c := range comments {
			userIDsMap[c.UserID] = true
			if len(c.Replies) > 0 {
				collectUserIDs(c.Replies)
			}
		}
	}
	collectUserIDs(comments)

	var userIDs []string
	for id := range userIDsMap {
		userIDs = append(userIDs, id)
	}

	userInfoMap, _ := userRepo.GetUsersByIDs(ctx, userIDs)

	var enrich func([]data.Comment)
	enrich = func(comments []data.Comment) {
		for i := range comments {
			if info, ok := userInfoMap[comments[i].UserID]; ok {
				comments[i].Username = info.Username
				comments[i].ProfilePictureURL = info.ProfilePictureURL
			}
			if len(comments[i].Replies) > 0 {
				enrich(comments[i].Replies)
			}
		}
	}
	enrich(comments)
}

// Helper to enrich comments with like info
func enrichCommentsWithLikeInfo(ctx context.Context, comments []data.Comment, likeRepo *data.LikeRepository, currentUserID string) {
	if likeRepo == nil || len(comments) == 0 {
		return
	}
	var commentIDs []string
	var collectCommentIDs func([]data.Comment)
	collectCommentIDs = func(comments []data.Comment) {
		for _, c := range comments {
			commentIDs = append(commentIDs, c.ID)
			if len(c.Replies) > 0 {
				collectCommentIDs(c.Replies)
			}
		}
	}
	collectCommentIDs(comments)

	likeInfoMap, _ := likeRepo.GetLikesForComments(ctx, commentIDs, currentUserID)

	var enrich func([]data.Comment)
	enrich = func(comments []data.Comment) {
		for i := range comments {
			if info, ok := likeInfoMap[comments[i].ID]; ok {
				comments[i].LikeCount = info.LikeCount
				comments[i].IsLiked = info.IsLiked
			}
			if len(comments[i].Replies) > 0 {
				enrich(comments[i].Replies)
			}
		}
	}
	enrich(comments)
}

// CreateComment handles POST /api/v1/posts/:id/comments
func CreateComment(commentRepo *data.CommentRepository, postRepo *data.PostRepository, notifRepo *data.NotificationRepository) gin.HandlerFunc {
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
				"error": "Invalid request body",
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
				err.Error() == "maximum comment depth of 3 reached" ||
				err.Error() == "cannot reply to a deleted comment" {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{
				"error": "Failed to create comment",
			})
			return
		}

		// Notify post author
		post, _ := postRepo.GetPostByID(c.Request.Context(), req.PostID)
		if post != nil && post.UserID != userID && notifRepo != nil {
			go notifRepo.CreateNotification(context.Background(), &data.CreateNotificationRequest{
				UserID:     post.UserID,
				Type:       data.NotificationTypeComment,
				ActorID:    userID,
				TargetType: data.TargetTypePost,
				TargetID:   req.PostID,
				Message:    "commented on your post",
			})
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Comment created",
			"comment": comment,
		})
	}
}


// EditComment handles PUT /api/v1/comments/:id
func EditComment(commentRepo *data.CommentRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		commentID := c.Param("id")
		userID := auth.GetUserID(c)

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		var req struct {
			Content string `json:"content" binding:"required,min=1,max=1000"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request body",
			})
			return
		}

		comment, err := commentRepo.EditComment(c.Request.Context(), commentID, userID, req.Content)
		if err != nil {
			status := http.StatusInternalServerError
			if err.Error() == "comment not found" {
				status = http.StatusNotFound
			} else if err.Error() == "unauthorized: can only edit your own comments" {
				status = http.StatusForbidden
			} else if err.Error() == "cannot edit a deleted comment" {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Comment updated",
			"comment": comment,
		})
	}
}

// GetComments handles GET /api/v1/posts/:id/comments
func GetComments(commentRepo *data.CommentRepository, userRepo *data.UserRepository, likeRepo *data.LikeRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		postID := c.Param("id")

		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil {
				limit = parsed
			}
		}

		cursor := c.Query("cursor")

		comments, nextCursor, hasMore, err := commentRepo.GetCommentsForPostPaginated(c.Request.Context(), postID, limit, cursor)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to get comments",
			})
			return
		}

		// Enrich
		ctx := c.Request.Context()
		currentUserID := auth.GetUserID(c)
		enrichCommentsWithUserInfo(ctx, comments, userRepo)
		enrichCommentsWithLikeInfo(ctx, comments, likeRepo, currentUserID)

		count, _ := commentRepo.GetCommentCount(ctx, postID)

		c.JSON(http.StatusOK, gin.H{
			"post_id":     postID,
			"total_count": count,
			"count":       len(comments),
			"data":        comments,
			"has_more":    hasMore,
			"next_cursor": nextCursor,
		})
	}
}

// GetReplies handles GET /api/v1/comments/:id/replies
func GetReplies(commentRepo *data.CommentRepository, userRepo *data.UserRepository, likeRepo *data.LikeRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		parentID := c.Param("id")

		limit := 10
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil {
				limit = parsed
			}
		}

		cursor := c.Query("cursor")

		replies, nextCursor, hasMore, err := commentRepo.GetRepliesForComment(c.Request.Context(), parentID, limit, cursor)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to get replies",
			})
			return
		}

		// Enrich
		ctx := c.Request.Context()
		currentUserID := auth.GetUserID(c)
		enrichCommentsWithUserInfo(ctx, replies, userRepo)
		enrichCommentsWithLikeInfo(ctx, replies, likeRepo, currentUserID)

		c.JSON(http.StatusOK, gin.H{
			"parent_id":   parentID,
			"count":       len(replies),
			"data":        replies,
			"has_more":    hasMore,
			"next_cursor": nextCursor,
		})
	}
}

// ReplyToComment handles POST /api/v1/comments/:id/reply
func ReplyToComment(commentRepo *data.CommentRepository, notifRepo *data.NotificationRepository) gin.HandlerFunc {
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

		if parent.IsDeleted {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot reply to a deleted comment"})
			return
		}

		var req data.CreateCommentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request body",
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
				"error": "Failed to create reply",
			})
			return
		}

		// Notify parent comment author
		if parent.UserID != userID && notifRepo != nil {
			go notifRepo.CreateNotification(context.Background(), &data.CreateNotificationRequest{
				UserID:     parent.UserID,
				Type:       data.NotificationTypeComment,
				ActorID:    userID,
				TargetType: data.TargetTypeComment,
				TargetID:   parentID,
				Message:    "replied to your comment",
			})
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
