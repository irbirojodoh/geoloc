package storage

import (
	"fmt"
	"os"
	"strings"
)

// StoredMediaValue returns the value to persist in Cassandra.
// Always returns the raw key to ensure we don't hardcode full domains in the DB.
func StoredMediaValue(store MediaStore, key string) string {
	return key
}

// ResolveMediaURL returns a usable URL for a stored key or URL value.
func ResolveMediaURL(store MediaStore, value string) string {
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	if !IsMediaKey(value) {
		return value
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return fmt.Sprintf("%s/api/v1/media/file?key=%s", strings.TrimSuffix(baseURL, "/"), value)
}

// ResolveMediaURLs resolves a slice of stored media values.
func ResolveMediaURLs(store MediaStore, values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if resolved := ResolveMediaURL(store, v); resolved != "" {
			out = append(out, resolved)
		}
	}
	return out
}
