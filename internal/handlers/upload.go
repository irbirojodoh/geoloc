package handlers

import (
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/storage"
)

// Allowed content types for upload
var allowedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

var allowedVideoTypes = map[string]bool{
	"video/mp4":       true,
	"video/quicktime": true,
}

// UploadAvatar handles POST /api/v1/upload/avatar
func UploadAvatar(store storage.Storage) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		file, header, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
			return
		}
		defer file.Close()

		// Check file size (max 5MB for avatars)
		if header.Size > 5*1024*1024 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File too large. Maximum 5MB"})
			return
		}

		// Check content type
		contentType := header.Header.Get("Content-Type")
		if !allowedImageTypes[contentType] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file type. Allowed: JPEG, PNG, GIF, WebP"})
			return
		}

		// Upload file
		filename, err := store.Upload(c.Request.Context(), "avatars", file, contentType)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to upload file",
				"details": err.Error(),
			})
			return
		}

		url := store.GetURL(filename)

		c.JSON(http.StatusOK, gin.H{
			"message":  "Avatar uploaded",
			"filename": filename,
			"url":      url,
		})
	}
}

// UploadPostMedia handles POST /api/v1/upload/post
func UploadPostMedia(store storage.Storage) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		file, header, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
			return
		}
		defer file.Close()

		// Check file size (max 50MB for posts)
		if header.Size > 50*1024*1024 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File too large. Maximum 50MB"})
			return
		}

		// Check content type
		contentType := header.Header.Get("Content-Type")
		isImage := allowedImageTypes[contentType]
		isVideo := allowedVideoTypes[contentType]

		if !isImage && !isVideo {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file type. Allowed: JPEG, PNG, GIF, WebP, MP4, MOV"})
			return
		}

		// Upload file
		filename, err := store.Upload(c.Request.Context(), "posts", file, contentType)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to upload file",
				"details": err.Error(),
			})
			return
		}

		url := store.GetURL(filename)
		mediaType := "image"
		if isVideo {
			mediaType = "video"
		}

		c.JSON(http.StatusOK, gin.H{
			"message":    "Media uploaded",
			"filename":   filename,
			"url":        url,
			"media_type": mediaType,
			"extension":  filepath.Ext(filename),
		})
	}
}
