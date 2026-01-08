package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/data"
)

// GetAddress handles GET /api/v1/geocode/address
// Returns address details for given coordinates.
// First checks location_names cache, then falls back to Nominatim.
func GetAddress(locRepo *data.LocationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Parse coordinates
		latStr := c.Query("lat")
		lngStr := c.Query("lng")

		if latStr == "" || lngStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "lat and lng query parameters are required",
			})
			return
		}

		lat, err := strconv.ParseFloat(latStr, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid latitude value",
			})
			return
		}

		lng, err := strconv.ParseFloat(lngStr, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid longitude value",
			})
			return
		}

		// Validate coordinate ranges
		if lat < -90 || lat > 90 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Latitude must be between -90 and 90",
			})
			return
		}

		if lng < -180 || lng > 180 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Longitude must be between -180 and 180",
			})
			return
		}

		// Get geohash prefix for this location
		geohashPrefix := data.GetGeohashPrefix(lat, lng)

		// GetOrFetch checks cache first, then calls Nominatim if needed
		locInfo, err := locRepo.GetOrFetch(c.Request.Context(), geohashPrefix, lat, lng)
		if err != nil {
			// Log the actual error for debugging
			fmt.Printf("[GEOCODE ERROR] lat=%f lng=%f geohash=%s error=%v\n", lat, lng, geohashPrefix, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to get address",
				"details": err.Error(),
			})
			return
		}

		// Check if locInfo is nil (should not happen, but handle gracefully)
		if locInfo == nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Address not found for this location",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"geohash":       geohashPrefix,
			"location_name": locInfo.Name,
			"address": gin.H{
				"village":       locInfo.Address.Village,
				"city_district": locInfo.Address.CityDistrict,
				"city":          locInfo.Address.City,
				"state":         locInfo.Address.State,
				"region":        locInfo.Address.Region,
				"postcode":      locInfo.Address.Postcode,
				"country":       locInfo.Address.Country,
				"country_code":  locInfo.Address.CountryCode,
			},
		})
	}
}
