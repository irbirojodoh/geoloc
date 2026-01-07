package handlers

import (
	"fmt"
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
func GetFeed(repo *data.PostRepository, userRepo *data.UserRepository, locRepo *data.LocationRepository) gin.HandlerFunc {
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

		// Decode cursor for pagination
		cursorTime, err := data.DecodeCursor(req.Cursor)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid cursor",
			})
			return
		}

		// Apply default limit
		limit := data.GetDefaultLimit(req.Limit, 20, 100)

		// Fetch one extra to determine if there are more posts
		posts, err := repo.GetNearbyPosts(
			c.Request.Context(),
			req.Latitude,
			req.Longitude,
			req.RadiusKM,
			limit+1,
			cursorTime,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to fetch feed",
				"details": err.Error(),
			})
			return
		}

		// Determine if there are more posts
		hasMore := len(posts) > limit
		if hasMore {
			posts = posts[:limit]
		}

		// Generate next cursor from last post's created_at
		var nextCursor string
		if hasMore && len(posts) > 0 {
			nextCursor = data.EncodeCursor(posts[len(posts)-1].CreatedAt)
		}

		// Enrich posts with user info and location info
		if len(posts) > 0 {
			// Collect unique user IDs
			userIDs := make([]string, 0, len(posts))
			seenUsers := make(map[string]bool)

			// Collect unique geohashes and their coordinates
			geohashes := make([]string, 0, len(posts))
			seenGeohashes := make(map[string]bool)
			latLngMap := make(map[string][2]float64)

			for _, p := range posts {
				if !seenUsers[p.UserID] {
					userIDs = append(userIDs, p.UserID)
					seenUsers[p.UserID] = true
				}
				geohashPrefix := data.GetGeohashPrefix(p.Latitude, p.Longitude)
				if !seenGeohashes[geohashPrefix] {
					geohashes = append(geohashes, geohashPrefix)
					seenGeohashes[geohashPrefix] = true
					latLngMap[geohashPrefix] = [2]float64{p.Latitude, p.Longitude}
				}
			}

			// Enrich with user info
			userInfoMap, _ := userRepo.GetUsersByIDs(c.Request.Context(), userIDs)
			for i := range posts {
				if info, ok := userInfoMap[posts[i].UserID]; ok {
					posts[i].Username = info.Username
					posts[i].ProfilePictureURL = info.ProfilePictureURL
				}
			}

			// Enrich with location info
			if locRepo != nil {
				locInfoMap, _ := locRepo.GetLocationsByGeohashes(c.Request.Context(), geohashes, latLngMap)
				for i := range posts {
					geohashPrefix := data.GetGeohashPrefix(posts[i].Latitude, posts[i].Longitude)
					if loc, ok := locInfoMap[geohashPrefix]; ok {
						posts[i].LocationName = loc.Name
						posts[i].Address = &loc.Address
					}
				}
			}
		}

		c.JSON(http.StatusOK, data.PaginatedResponse{
			Data:       posts,
			Count:      len(posts),
			HasMore:    hasMore,
			NextCursor: nextCursor,
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
func GetUserPosts(repo *data.PostRepository, userRepo *data.UserRepository, locRepo *data.LocationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("id")

		// Get user info first
		user, err := userRepo.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "User not found",
			})
			return
		}

		// Parse query parameters
		cursor := c.Query("cursor")
		limitStr := c.DefaultQuery("limit", "20")
		var limit int
		fmt.Sscanf(limitStr, "%d", &limit)

		// Decode cursor for pagination
		cursorTime, err := data.DecodeCursor(cursor)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid cursor",
			})
			return
		}

		// Apply default limit
		limit = data.GetDefaultLimit(limit, 20, 100)

		// Fetch one extra to determine if there are more posts
		posts, err := repo.GetPostsByUser(c.Request.Context(), userID, limit+1, cursorTime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to fetch user posts",
				"details": err.Error(),
			})
			return
		}

		// Determine if there are more posts
		hasMore := len(posts) > limit
		if hasMore {
			posts = posts[:limit]
		}

		// Generate next cursor from last post's created_at
		var nextCursor string
		if hasMore && len(posts) > 0 {
			nextCursor = data.EncodeCursor(posts[len(posts)-1].CreatedAt)
		}

		// Enrich posts with location info
		if locRepo != nil && len(posts) > 0 {
			geohashes := make([]string, 0, len(posts))
			latLngMap := make(map[string][2]float64)

			for _, p := range posts {
				geohashPrefix := data.GetGeohashPrefix(p.Latitude, p.Longitude)
				geohashes = append(geohashes, geohashPrefix)
				latLngMap[geohashPrefix] = [2]float64{p.Latitude, p.Longitude}
			}

			locInfoMap, _ := locRepo.GetLocationsByGeohashes(c.Request.Context(), geohashes, latLngMap)
			for i := range posts {
				geohashPrefix := data.GetGeohashPrefix(posts[i].Latitude, posts[i].Longitude)
				if loc, ok := locInfoMap[geohashPrefix]; ok {
					posts[i].LocationName = loc.Name
					posts[i].Address = &loc.Address
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"user": gin.H{
				"id":                  user.ID,
				"username":            user.Username,
				"full_name":           user.FullName,
				"profile_picture_url": user.ProfilePictureURL,
			},
			"count":       len(posts),
			"data":        posts,
			"has_more":    hasMore,
			"next_cursor": nextCursor,
		})
	}
}
