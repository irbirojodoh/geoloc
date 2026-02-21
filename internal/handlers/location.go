package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
)

// FollowLocation handles POST /api/v1/locations/follow
func FollowLocation(locRepo *data.LocationFollowRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		var req data.FollowLocationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request body",
			})
			return
		}

		// Validate coordinates
		if req.Latitude < -90 || req.Latitude > 90 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Latitude must be between -90 and 90"})
			return
		}
		if req.Longitude < -180 || req.Longitude > 180 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Longitude must be between -180 and 180"})
			return
		}

		location, err := locRepo.FollowLocation(c.Request.Context(), userID, &req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to follow location",
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message":  "Location followed",
			"location": location,
		})
	}
}

// UnfollowLocation handles DELETE /api/v1/locations/:geohash/follow
func UnfollowLocation(locRepo *data.LocationFollowRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		geohash := c.Param("geohash")

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		err := locRepo.UnfollowLocation(c.Request.Context(), userID, geohash)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Location unfollowed"})
	}
}

// GetFollowedLocations handles GET /api/v1/locations/following
func GetFollowedLocations(locRepo *data.LocationFollowRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		locations, err := locRepo.GetFollowedLocations(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"locations": locations,
			"count":     len(locations),
		})
	}
}
