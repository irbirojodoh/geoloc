package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/data"
)

// CreatePost handles POST /api/v1/posts
func CreatePost(repo *data.PostRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req data.CreatePostRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request body",
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

		post, err := repo.CreatePost(c.Request.Context(), &req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to create post",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Post created successfully",
			"post": post,
		})
	}
}

// GetFeed handles GET /api/v1/feed
func GetFeed(repo *data.PostRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req data.GetFeedRequest

		if err := c.ShouldBindQuery(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid query parameters",
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
				"error": "Failed to fetch feed",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Feed fetched successfully",
			"count": len(posts),
			"posts": posts,
		})
	}
}
