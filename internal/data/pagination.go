package data

import (
	"encoding/base64"
	"fmt"
	"time"
)

// Pagination represents cursor-based pagination parameters
type Pagination struct {
	Cursor string `form:"cursor"`
	Limit  int    `form:"limit"`
}

// PaginatedResponse represents a paginated response
type PaginatedResponse struct {
	Data       any    `json:"data"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
	Count      int    `json:"count"`
}

// EncodeCursor encodes a timestamp into a cursor string
func EncodeCursor(t time.Time) string {
	return base64.StdEncoding.EncodeToString([]byte(t.Format(time.RFC3339Nano)))
}

// DecodeCursor decodes a cursor string into a timestamp
func DecodeCursor(cursor string) (time.Time, error) {
	if cursor == "" {
		return time.Time{}, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cursor")
	}

	t, err := time.Parse(time.RFC3339Nano, string(decoded))
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cursor format")
	}

	return t, nil
}

// GetDefaultLimit returns the limit with defaults applied
func GetDefaultLimit(limit, defaultLimit, maxLimit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// NewPaginatedResponse creates a new paginated response
func NewPaginatedResponse(data any, count int, hasMore bool, nextCursor string) PaginatedResponse {
	return PaginatedResponse{
		Data:       data,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Count:      count,
	}
}
