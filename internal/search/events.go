package search

import (
	"regexp"
	"strings"
	"time"
)

// UserIndexedEvent is published when a user should be indexed or re-indexed in Elasticsearch.
type UserIndexedEvent struct {
	UserID        string `json:"user_id"`
	Username      string `json:"username"`
	DisplayName   string `json:"display_name"`
	FollowerCount int    `json:"follower_count"`
	IsVerified    bool   `json:"is_verified"`
	AvatarURL     string `json:"avatar_url,omitempty"`
}

// PostCreatedEvent is published when a new post is created for search indexing.
type PostCreatedEvent struct {
	PostID    string    `json:"post_id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	Hashtags  []string  `json:"hashtags"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	Geohash   string    `json:"geohash"`
	CreatedAt time.Time `json:"created_at"`
	LikeCount int       `json:"like_count"`
}

var hashtagPattern = regexp.MustCompile(`#([A-Za-z0-9_]+)`)

// ExtractHashtags returns lowercase hashtag tokens without the leading '#'.
func ExtractHashtags(content string) []string {
	matches := hashtagPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	hashtags := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		tag := strings.ToLower(match[1])
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		hashtags = append(hashtags, tag)
	}
	return hashtags
}
