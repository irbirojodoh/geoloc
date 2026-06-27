package storage

import (
	"strings"
	"time"
)

// PresignGetExpiry is how long presigned GET URLs remain valid in API responses.
const PresignGetExpiry = 15 * time.Minute

// StoredMediaValue returns the value to persist in Cassandra.
// Always returns the raw key to ensure we don't hardcode full domains in the DB.
func StoredMediaValue(store MediaStore, key string) string {
	return key
}

// ResolveMediaURL returns a presigned GET URL for a stored R2 key, or the value unchanged for external URLs.
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
	if store == nil {
		return ""
	}
	url, err := store.PresignGetURL(value, PresignGetExpiry)
	if err != nil {
		return ""
	}
	return url
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
