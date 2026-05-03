package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
)

// RegisterDeviceRequest represents the request to register a device token
type RegisterDeviceRequest struct {
	Token    string `json:"token" binding:"required"`
	Platform string `json:"platform" binding:"required,oneof=ios android web"`
}

// RegisterDevice handles POST /api/v1/devices
func RegisterDevice(deviceRepo *data.DeviceRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		var req RegisterDeviceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request body",
			})
			return
		}

		err := deviceRepo.RegisterDevice(c.Request.Context(), userID, req.Token, req.Platform)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register device"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Device registered",
		})
	}
}

// UnregisterDevice handles DELETE /api/v1/devices
func UnregisterDevice(deviceRepo *data.DeviceRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		var req struct {
			Token string `json:"token" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Token is required"})
			return
		}

		err := deviceRepo.UnregisterDevice(c.Request.Context(), userID, req.Token)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unregister device"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Device unregistered",
		})
	}
}
