package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Storage defines the interface for file storage
type Storage interface {
	Upload(ctx context.Context, filename string, content io.Reader, contentType string) (string, error)
	Delete(ctx context.Context, filename string) error
	GetURL(filename string) string
}

// LocalStorage implements Storage for local filesystem
type LocalStorage struct {
	BasePath string
	BaseURL  string
}

// NewLocalStorage creates a new local storage instance
func NewLocalStorage(basePath, baseURL string) *LocalStorage {
	// Create upload directories if they don't exist
	os.MkdirAll(filepath.Join(basePath, "avatars"), 0755)
	os.MkdirAll(filepath.Join(basePath, "posts"), 0755)

	return &LocalStorage{
		BasePath: basePath,
		BaseURL:  baseURL,
	}
}

// Upload saves a file to local storage
func (s *LocalStorage) Upload(ctx context.Context, folder string, content io.Reader, contentType string) (string, error) {
	// Generate unique filename
	ext := getExtensionFromContentType(contentType)
	filename := fmt.Sprintf("%s_%d%s", uuid.New().String(), time.Now().Unix(), ext)

	fullPath := filepath.Join(s.BasePath, folder, filename)

	file, err := os.Create(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, content)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return filepath.Join(folder, filename), nil
}

// Delete removes a file from local storage
func (s *LocalStorage) Delete(ctx context.Context, filename string) error {
	fullPath := filepath.Join(s.BasePath, filename)
	return os.Remove(fullPath)
}

// GetURL returns the URL for a file
func (s *LocalStorage) GetURL(filename string) string {
	return s.BaseURL + "/" + filename
}

func getExtensionFromContentType(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	default:
		return ".bin"
	}
}

// S3Storage implements Storage for AWS S3/compatible services
// Template for future use - stores config but uses local fallback
type S3Storage struct {
	Bucket     string
	Region     string
	AccessKey  string
	SecretKey  string
	Endpoint   string // For S3-compatible services like Cloudflare R2
	CDNBaseURL string
	local      *LocalStorage // Fallback
}

// NewS3Storage creates a new S3 storage instance
// Currently falls back to local storage
func NewS3Storage(bucket, region, accessKey, secretKey, endpoint, cdnURL string, localFallback *LocalStorage) *S3Storage {
	return &S3Storage{
		Bucket:     bucket,
		Region:     region,
		AccessKey:  accessKey,
		SecretKey:  secretKey,
		Endpoint:   endpoint,
		CDNBaseURL: cdnURL,
		local:      localFallback,
	}
}

// Upload to S3 (currently uses local fallback)
func (s *S3Storage) Upload(ctx context.Context, folder string, content io.Reader, contentType string) (string, error) {
	// TODO: Implement actual S3 upload when ready
	// For now, use local storage
	if s.local != nil {
		return s.local.Upload(ctx, folder, content, contentType)
	}
	return "", fmt.Errorf("S3 not configured and no local fallback")
}

// Delete from S3 (currently uses local fallback)
func (s *S3Storage) Delete(ctx context.Context, filename string) error {
	if s.local != nil {
		return s.local.Delete(ctx, filename)
	}
	return fmt.Errorf("S3 not configured and no local fallback")
}

// GetURL returns the CDN URL for a file
func (s *S3Storage) GetURL(filename string) string {
	if s.CDNBaseURL != "" {
		return s.CDNBaseURL + "/" + filename
	}
	if s.local != nil {
		return s.local.GetURL(filename)
	}
	return filename
}
