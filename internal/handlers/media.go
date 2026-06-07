package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
	"social-geo-go/internal/storage"
)

const (
	mediaSignExpiry   = 15 * time.Minute
	mediaUploadExpiry = 10 * time.Minute
)

// MediaHandler serves authenticated media access endpoints.
type MediaHandler struct {
	Store storage.MediaStore
}

// RegisterMediaRoutes mounts /media routes on the protected API group.
func RegisterMediaRoutes(api *gin.RouterGroup, h *MediaHandler) {
	media := api.Group("/media")
	{
		media.GET("/sign", h.SignURL)
		media.POST("/upload-url", h.UploadURL)
		media.DELETE("/object", h.DeleteObject)
	}
}

// SignURL handles GET /api/v1/media/sign?key=...
func (h *MediaHandler) SignURL(c *gin.Context) {
	key := strings.TrimSpace(c.Query("key"))
	if err := storage.ValidateKey(key); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key"})
		return
	}

	expiresAt := time.Now().UTC().Add(mediaSignExpiry)
	url, err := h.Store.PresignGetURL(key, mediaSignExpiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sign URL"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":        url,
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}

type uploadURLRequest struct {
	Folder      string `json:"folder" binding:"required"`
	ContentType string `json:"content_type" binding:"required"`
	Filename    string `json:"filename"`
}

// UploadURL handles POST /api/v1/media/upload-url
func (h *MediaHandler) UploadURL(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req uploadURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if !storage.AllowedFolders[req.Folder] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid folder"})
		return
	}
	if !storage.AllowedImageContentTypes[req.ContentType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid content type"})
		return
	}

	key, err := storage.GenerateKey(req.Folder, userID, req.Filename, req.ContentType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	expiresAt := time.Now().UTC().Add(mediaUploadExpiry)
	uploadURL, err := h.Store.PresignPutURL(key, mediaUploadExpiry, req.ContentType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload URL"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"upload_url": uploadURL,
		"key":        key,
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}

// DeleteObject handles DELETE /api/v1/media/object?key=...
func (h *MediaHandler) DeleteObject(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	key := strings.TrimSpace(c.Query("key"))
	if err := storage.ValidateKey(key); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key"})
		return
	}

	ownerID, err := storage.KeyOwnerUserID(key)
	if err != nil || ownerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not allowed to delete this object"})
		return
	}

	if err := h.Store.DeleteObject(c.Request.Context(), key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete object"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Object deleted"})
}

// ResolveUserMediaURLs populates resolved media URLs on a user for API responses.
func ResolveUserMediaURLs(store storage.MediaStore, user *data.User) {
	if user == nil || store == nil {
		return
	}
	user.ProfilePictureURL = storage.ResolveMediaURL(store, user.ProfilePictureURL)
	if user.CoverImageURL != "" && user.CoverImageURL != data.DefaultCoverImageURL {
		user.CoverImageURL = storage.ResolveMediaURL(store, user.CoverImageURL)
	}
}

// ResolvePostMediaURLs populates resolved media URLs on a post for API responses.
func ResolvePostMediaURLs(store storage.MediaStore, post *data.Post) {
	if post == nil || store == nil {
		return
	}
	post.MediaURLs = storage.ResolveMediaURLs(store, post.MediaURLs)
	if post.ProfilePictureURL != "" {
		post.ProfilePictureURL = storage.ResolveMediaURL(store, post.ProfilePictureURL)
	}
}

// ResolvePostsMediaURLs resolves media URLs for a slice of posts.
func ResolvePostsMediaURLs(store storage.MediaStore, posts []data.Post) {
	for i := range posts {
		ResolvePostMediaURLs(store, &posts[i])
	}
}