package storage

import (
	"strings"
	"time"
)

// StoredMediaValue returns the value to persist in Cassandra.
// Uses the public CDN URL when configured, otherwise the raw object key.
func StoredMediaValue(store MediaStore, key string) string {
	if key == "" {
		return ""
	}
	if store != nil {
		if url := store.PublicURL(key); url != "" {
			return url
		}
	}
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
	if store == nil {
		return ""
	}
	if url := store.PublicURL(value); url != "" {
		return url
	}
	signed, err := store.PresignGetURL(value, time.Hour)
	if err != nil {
		return ""
	}
	return signed
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
