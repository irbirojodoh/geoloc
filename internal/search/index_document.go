package search

import "time"

// PostDocument is the Elasticsearch document shape for a post.
type PostDocument struct {
	PostID    string    `json:"post_id"`
	UserID    string    `json:"user_id"`
	Content   string    `json:"content"`
	Hashtags  []string  `json:"hashtags,omitempty"`
	Location  *GeoPoint `json:"location,omitempty"`
	Geohash   string    `json:"geohash"`
	CreatedAt time.Time `json:"created_at"`
	LikeCount int       `json:"like_count"`
}

type GeoPoint struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// PostDocumentFromEvent builds an ES document from a post-created event.
func PostDocumentFromEvent(event PostCreatedEvent) PostDocument {
	doc := PostDocument{
		PostID:    event.PostID,
		UserID:    event.UserID,
		Content:   event.Content,
		Hashtags:  event.Hashtags,
		Geohash:   event.Geohash,
		CreatedAt: event.CreatedAt,
		LikeCount: event.LikeCount,
	}
	if event.Lat != 0 || event.Lon != 0 {
		doc.Location = &GeoPoint{Lat: event.Lat, Lon: event.Lon}
	}
	return doc
}
