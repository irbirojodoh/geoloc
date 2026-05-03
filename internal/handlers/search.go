package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/data"
)

// SearchUsers handles GET /api/v1/search/users
func SearchUsers(userRepo *data.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := strings.TrimSpace(c.Query("q"))
		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Search query is required"})
			return
		}

		if len(query) < 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Search query must be at least 2 characters"})
			return
		}

		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
				limit = parsed
			}
		}

		users, err := userRepo.SearchUsers(c.Request.Context(), query, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Search failed"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"query":   query,
			"results": users,
			"count":   len(users),
		})
	}
}

// SearchPosts handles GET /api/v1/search/posts
func SearchPosts(postRepo *data.PostRepository, userRepo *data.UserRepository, likeRepo *data.LikeRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := strings.TrimSpace(c.Query("q"))
		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Search query is required"})
			return
		}

		if len(query) < 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Search query must be at least 2 characters"})
			return
		}

		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
				limit = parsed
			}
		}

		posts, err := postRepo.SearchPosts(c.Request.Context(), query, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Search failed"})
			return
		}

		// Enrich posts with author info
		if len(posts) > 0 {
			postIDs := make([]string, 0, len(posts))
			userIDs := make([]string, 0, len(posts))
			for _, p := range posts {
				userIDs = append(userIDs, p.UserID)
				postIDs = append(postIDs, p.ID)
			}
			userInfoMap, _ := userRepo.GetUsersByIDs(c.Request.Context(), userIDs)
			
			currentUserID, _ := c.Get("user_id")
			var uid string
			if id, ok := currentUserID.(string); ok {
				uid = id
			}

			likeInfo, _ := likeRepo.GetLikesForPosts(c.Request.Context(), postIDs, uid)
			
			for i := range posts {
				if info, ok := userInfoMap[posts[i].UserID]; ok {
					posts[i].Username = info.Username
					posts[i].ProfilePictureURL = info.ProfilePictureURL
				}
				// Enrich like state
				if info, ok := likeInfo[posts[i].ID]; ok {
					posts[i].IsLiked = info.IsLiked
					posts[i].LikeCount = info.LikeCount
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"query":   query,
			"results": posts,
			"count":   len(posts),
		})
	}
}
