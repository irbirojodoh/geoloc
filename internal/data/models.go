package data

import (
	"time"

	"github.com/google/uuid"
)

// User represents a user in the system
type User struct {
	ID                string     `json:"id"`
	Username          string     `json:"username"`
	Email             string     `json:"email"`
	FullName          string     `json:"full_name,omitempty"`
	Bio               string     `json:"bio,omitempty"`
	PhoneNumber       string     `json:"phone_number,omitempty"`
	ProfilePictureURL string     `json:"profile_picture_url,omitempty"`
	PasswordHash      string     `json:"-"`
	LastOnline        *time.Time `json:"last_online,omitempty"`
	LastIPAddress     string     `json:"-"` // Don't expose in JSON
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
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
	ID                string   `json:"id"`
	UserID            string   `json:"user_id"`
	Username          string   `json:"username,omitempty"`
	ProfilePictureURL string   `json:"profile_picture_url,omitempty"`
	Content           string   `json:"content"`
	MediaURLs         []string `json:"media_urls,omitempty"`
	Latitude          float64  `json:"-"` // Hidden - use geohash instead
	Longitude         float64  `json:"-"` // Hidden - use geohash instead
	Geohash           string   `json:"geohash,omitempty"`
	// Location info (from cached geocoding)
	LocationName string           `json:"location_name,omitempty"`
	Address      *LocationAddress `json:"address,omitempty"`
	IPAddress    string           `json:"-"` // Don't expose in JSON
	UserAgent    string           `json:"-"` // Don't expose in JSON
	CreatedAt    time.Time        `json:"created_at"`
	Distance     float64          `json:"distance_km,omitempty"`
}

// CreatePostRequest represents the request body for creating a post
type CreatePostRequest struct {
	UserID    string   `json:"user_id" binding:"required"`
	Content   string   `json:"content" binding:"required"`
	MediaURLs []string `json:"media_urls"` // Max 4 URLs
	Latitude  float64  `json:"latitude" binding:"required"`
	Longitude float64  `json:"longitude" binding:"required"`
	IPAddress string   `json:"-"` // Set from request context
	UserAgent string   `json:"-"` // Set from request context
}

// GetFeedRequest represents query parameters for fetching feed
type GetFeedRequest struct {
	Latitude  float64 `form:"latitude" binding:"required"`
	Longitude float64 `form:"longitude" binding:"required"`
	RadiusKM  float64 `form:"radius_km"`
	Limit     int     `form:"limit"`
	Cursor    string  `form:"cursor"`
}

// ValidateMediaURLs checks if media URLs array has max 4 items
func (r *CreatePostRequest) ValidateMediaURLs() bool {
	return len(r.MediaURLs) <= 4
}

// GenerateUUID creates a new UUID string
func GenerateUUID() string {
	return uuid.New().String()
}

// ============== LIKES ==============

// Like represents a like on a post or comment
type Like struct {
	TargetType string    `json:"target_type"` // "post" or "comment"
	TargetID   string    `json:"target_id"`
	UserID     string    `json:"user_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// LikeRequest represents a like action request
type LikeRequest struct {
	TargetType string `json:"-"` // Set from route
	TargetID   string `json:"-"` // Set from route
	UserID     string `json:"-"` // Set from auth context
}

// ============== COMMENTS ==============

// Comment represents a comment on a post or reply to another comment
type Comment struct {
	ID        string    `json:"id"`
	PostID    string    `json:"post_id"`
	ParentID  string    `json:"parent_id,omitempty"` // null for top-level
	UserID    string    `json:"user_id"`
	Content   string    `json:"content"`
	Depth     int       `json:"depth"` // 1, 2, or 3
	LikeCount int64     `json:"like_count"`
	IPAddress string    `json:"-"`
	UserAgent string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	Replies   []Comment `json:"replies,omitempty"` // Nested replies
}

// CreateCommentRequest represents the request body for creating a comment
type CreateCommentRequest struct {
	PostID    string `json:"-"`                   // Set from route
	ParentID  string `json:"parent_id,omitempty"` // Optional parent comment
	UserID    string `json:"-"`                   // Set from auth context
	Content   string `json:"content" binding:"required,min=1,max=1000"`
	Depth     int    `json:"-"` // Set based on parent
	IPAddress string `json:"-"`
	UserAgent string `json:"-"`
}

const (
	MaxCommentDepth   = 3
	TargetTypePost    = "post"
	TargetTypeComment = "comment"
)

// ============== PROFILE ==============

// UpdateProfileRequest represents the request body for updating a profile
type UpdateProfileRequest struct {
	FullName          string `json:"full_name"`
	Bio               string `json:"bio"`
	PhoneNumber       string `json:"phone_number"`
	ProfilePictureURL string `json:"profile_picture_url"`
}

// ============== FOLLOWS ==============

// Follow represents a follow relationship
type Follow struct {
	FollowerID  string    `json:"follower_id"`
	FollowingID string    `json:"following_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// FollowCounts represents follower/following counts
type FollowCounts struct {
	UserID         string `json:"user_id"`
	FollowersCount int64  `json:"followers_count"`
	FollowingCount int64  `json:"following_count"`
}

// ============== LOCATION FOLLOWS ==============

// LocationFollow represents a user following a geographic area
type LocationFollow struct {
	UserID        string    `json:"user_id"`
	GeohashPrefix string    `json:"geohash_prefix"`
	Name          string    `json:"name"`
	Latitude      float64   `json:"latitude"`
	Longitude     float64   `json:"longitude"`
	CreatedAt     time.Time `json:"created_at"`
}

// FollowLocationRequest represents the request body for following a location
type FollowLocationRequest struct {
	Name      string  `json:"name" binding:"required"`
	Latitude  float64 `json:"latitude" binding:"required"`
	Longitude float64 `json:"longitude" binding:"required"`
}

// ============== NOTIFICATIONS ==============

const (
	NotificationTypeLike         = "like"
	NotificationTypeComment      = "comment"
	NotificationTypeFollow       = "follow"
	NotificationTypeLocationPost = "location_post"
)

// Notification represents a user notification
type Notification struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Type       string    `json:"type"`
	ActorID    string    `json:"actor_id"`
	TargetType string    `json:"target_type,omitempty"`
	TargetID   string    `json:"target_id,omitempty"`
	Message    string    `json:"message"`
	IsRead     bool      `json:"is_read"`
	CreatedAt  time.Time `json:"created_at"`
}

// CreateNotificationRequest represents the request to create a notification
type CreateNotificationRequest struct {
	UserID     string
	Type       string
	ActorID    string
	TargetType string
	TargetID   string
	Message    string
}
