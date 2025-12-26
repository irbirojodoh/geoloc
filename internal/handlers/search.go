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
			if parsed, err := strconv.Atoi(l); err == nil {
				limit = parsed
			}
		}

		users, err := userRepo.SearchUsers(c.Request.Context(), query, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
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
func SearchPosts(postRepo *data.PostRepository) gin.HandlerFunc {
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
			if parsed, err := strconv.Atoi(l); err == nil {
				limit = parsed
			}
		}

		posts, err := postRepo.SearchPosts(c.Request.Context(), query, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"query":   query,
			"results": posts,
			"count":   len(posts),
		})
	}
}
