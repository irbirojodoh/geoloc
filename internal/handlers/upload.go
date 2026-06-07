package handlers

import (
	"io"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/storage"
)

// UploadAvatar handles POST /api/v1/upload/avatar (Pattern A — server-side multipart upload).
func UploadAvatar(store storage.MediaStore) gin.HandlerFunc {
	return uploadImage(store, "avatars", "Avatar uploaded")
}

// UploadCover handles POST /api/v1/upload/cover (Pattern A — server-side multipart upload).
func UploadCover(store storage.MediaStore) gin.HandlerFunc {
	return uploadImage(store, "covers", "Cover image uploaded")
}

// UploadPostMedia handles POST /api/v1/upload/post (Pattern A — server-side multipart upload).
func UploadPostMedia(store storage.MediaStore) gin.HandlerFunc {
	return uploadImage(store, "posts", "Media uploaded")
}

func uploadImage(store storage.MediaStore, folder, message string) gin.HandlerFunc {
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

		if header.Size > storage.MaxUploadSize {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File too large. Maximum 10MB"})
			return
		}

		contentType, err := detectImageContentType(file)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read file"})
			return
		}
		if !storage.AllowedImageContentTypes[contentType] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file type. Allowed: JPEG, PNG, GIF, WebP"})
			return
		}

		key, err := storage.GenerateKey(folder, userID, header.Filename, contentType)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := store.PutObject(c.Request.Context(), key, file, header.Size, contentType); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload file"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": message,
			"key":     key,
			"url":     storage.ResolveMediaURL(store, key),
		})
	}
}

func detectImageContentType(file multipart.File) (string, error) {
	var buf [512]byte
	n, err := file.Read(buf[:])
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return http.DetectContentType(buf[:n]), nil
}
