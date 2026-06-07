package storage

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const MaxUploadSize = 10 * 1024 * 1024 // 10MB

var (
	AllowedFolders = map[string]bool{
		"avatars": true,
		"covers":  true,
		"posts":   true,
	}

	AllowedImageContentTypes = map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/webp": true,
		"image/gif":  true,
	}
)

// GenerateKey builds {folder}/{userId}/{uuid}{ext}.
func GenerateKey(folder, userID, filename, contentType string) (string, error) {
	if !AllowedFolders[folder] {
		return "", fmt.Errorf("invalid folder: %s", folder)
	}
	if userID == "" {
		return "", fmt.Errorf("user ID is required")
	}
	if !AllowedImageContentTypes[contentType] {
		return "", fmt.Errorf("invalid content type: %s", contentType)
	}

	ext := ExtensionFromFilenameOrContentType(filename, contentType)
	return fmt.Sprintf("%s/%s/%s%s", folder, userID, uuid.New().String(), ext), nil
}

// ExtensionFromFilenameOrContentType returns a file extension for the upload.
func ExtensionFromFilenameOrContentType(filename, contentType string) string {
	if ext := filepath.Ext(filename); ext != "" {
		return strings.ToLower(ext)
	}
	return extensionFromContentType(contentType)
}

func extensionFromContentType(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}

// ValidateKey ensures the object key is safe and uses an allowed prefix.
func ValidateKey(key string) error {
	if key == "" {
		return fmt.Errorf("key is required")
	}
	if strings.Contains(key, "..") {
		return fmt.Errorf("invalid key")
	}
	parts := strings.Split(key, "/")
	if len(parts) != 3 {
		return fmt.Errorf("invalid key format")
	}
	if !AllowedFolders[parts[0]] {
		return fmt.Errorf("invalid key prefix")
	}
	if parts[1] == "" || parts[2] == "" {
		return fmt.Errorf("invalid key format")
	}
	return nil
}

// KeyOwnerUserID returns the user ID embedded in the key (second segment).
func KeyOwnerUserID(key string) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	return strings.Split(key, "/")[1], nil
}

// IsMediaKey reports whether value looks like an R2 object key we manage.
func IsMediaKey(value string) bool {
	if value == "" {
		return false
	}
	for folder := range AllowedFolders {
		if strings.HasPrefix(value, folder+"/") {
			return true
		}
	}
	return false
}

// KeyFromStoredValue extracts an object key from a stored DB value (key or public URL).
func KeyFromStoredValue(value, publicDomain string) string {
	if value == "" {
		return ""
	}
	if IsMediaKey(value) {
		return value
	}
	if publicDomain != "" {
		base := strings.TrimSuffix(publicDomain, "/")
		if strings.HasPrefix(value, base+"/") {
			return strings.TrimPrefix(value, base+"/")
		}
	}
	return ""
}
