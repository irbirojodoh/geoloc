package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/data"
)

// CreatePost handles POST /api/v1/posts
func CreatePost(postRepo *data.PostRepository, userRepo *data.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req data.CreatePostRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request body",
				"details": err.Error(),
			})
			return
		}

		// Validate coordinates
		if req.Latitude < -90 || req.Latitude > 90 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Latitude must be between -90 and 90",
			})
			return
		}

		if req.Longitude < -180 || req.Longitude > 180 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Longitude must be between -180 and 180",
			})
			return
		}

		// Validate media URLs (max 4)
		if !req.ValidateMediaURLs() {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Maximum 4 media URLs allowed",
			})
			return
		}

		// Validate user exists
		exists, err := userRepo.UserExists(c.Request.Context(), req.UserID)
		if err != nil || !exists {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "User not found",
			})
			return
		}

		// Capture IP address and user agent for tracking
		req.IPAddress = c.ClientIP()
		req.UserAgent = c.Request.UserAgent()

		post, err := postRepo.CreatePost(c.Request.Context(), &req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to create post",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Post created successfully",
			"post":    post,
		})
	}
}

// GetFeed handles GET /api/v1/feed
func GetFeed(repo *data.PostRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req data.GetFeedRequest

		if err := c.ShouldBindQuery(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid query parameters",
				"details": err.Error(),
			})
			return
		}

		// Validate coordinates
		if req.Latitude < -90 || req.Latitude > 90 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Latitude must be between -90 and 90",
			})
			return
		}

		if req.Longitude < -180 || req.Longitude > 180 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Longitude must be between -180 and 180",
			})
			return
		}

		posts, err := repo.GetNearbyPosts(
			c.Request.Context(),
			req.Latitude,
			req.Longitude,
			req.RadiusKM,
			req.Limit,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to fetch feed",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Feed fetched successfully",
			"count":   len(posts),
			"posts":   posts,
		})
	}
}

// GetPost handles GET /api/v1/posts/:id
func GetPost(repo *data.PostRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		post, err := repo.GetPostByID(c.Request.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				c.JSON(http.StatusNotFound, gin.H{
					"error": "Post not found",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to fetch post",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"post": post,
		})
	}
}

// GetUserPosts handles GET /api/v1/users/:id/posts
func GetUserPosts(repo *data.PostRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("id")

		posts, err := repo.GetPostsByUser(c.Request.Context(), userID, 50)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to fetch user posts",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"count": len(posts),
			"posts": posts,
		})
	}
}
