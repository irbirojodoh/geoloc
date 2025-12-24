package data

import (
	"time"

	"github.com/google/uuid"
)

// User represents a user in the system
type User struct {
	ID                string    `json:"id"`
	Username          string    `json:"username"`
	Email             string    `json:"email"`
	FullName          string    `json:"full_name,omitempty"`
	Bio               string    `json:"bio,omitempty"`
	PhoneNumber       string    `json:"phone_number,omitempty"`
	ProfilePictureURL string    `json:"profile_picture_url,omitempty"`
	PasswordHash      string    `json:"-"` // Never expose in JSON
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// CreateUserRequest represents the request body for creating a user
type CreateUserRequest struct {
	Username          string `json:"username" binding:"required,min=3,max=50"`
	Email             string `json:"email" binding:"required,email"`
	FullName          string `json:"full_name"`
	Bio               string `json:"bio"`
	PhoneNumber       string `json:"phone_number"`
	ProfilePictureURL string `json:"profile_picture_url"`
	PasswordHash      string `json:"-"` // Set internally, not from request
}

// Post represents a social media post with geospatial data
type Post struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Content   string    `json:"content"`
	MediaURLs []string  `json:"media_urls,omitempty"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Geohash   string    `json:"geohash,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Distance  float64   `json:"distance_km,omitempty"` // Calculated field for feed
}

// CreatePostRequest represents the request body for creating a post
type CreatePostRequest struct {
	UserID    string   `json:"user_id" binding:"required"`
	Content   string   `json:"content" binding:"required"`
	MediaURLs []string `json:"media_urls"` // Max 4 URLs
	Latitude  float64  `json:"latitude" binding:"required"`
	Longitude float64  `json:"longitude" binding:"required"`
}

// GetFeedRequest represents query parameters for fetching feed
type GetFeedRequest struct {
	Latitude  float64 `form:"latitude" binding:"required"`
	Longitude float64 `form:"longitude" binding:"required"`
	RadiusKM  float64 `form:"radius_km"`
	Limit     int     `form:"limit"`
}

// ValidateMediaURLs checks if media URLs array has max 4 items
func (r *CreatePostRequest) ValidateMediaURLs() bool {
	return len(r.MediaURLs) <= 4
}

// GenerateUUID creates a new UUID string
func GenerateUUID() string {
	return uuid.New().String()
}
