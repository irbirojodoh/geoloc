package data

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gocql/gocql"
)

type PostRepository struct {
	session *gocql.Session
}

func NewPostRepository(session *gocql.Session) *PostRepository {
	return &PostRepository{session: session}
}

// CreatePost inserts a new post into all denormalized tables
func (r *PostRepository) CreatePost(ctx context.Context, req *CreatePostRequest) (*Post, error) {
	postID := gocql.TimeUUID()
	userID, err := gocql.ParseUUID(req.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	now := time.Now()
	fullGeohash := EncodeGeohash(req.Latitude, req.Longitude, 7)
	geohashPrefix := GetGeohashPrefix(req.Latitude, req.Longitude)

	// Insert into posts_by_geohash
	err = r.session.Query(`
		INSERT INTO posts_by_geohash (geohash_prefix, created_at, post_id, user_id, content, media_urls, latitude, longitude, full_geohash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, geohashPrefix, now, postID, userID, req.Content, req.MediaURLs, req.Latitude, req.Longitude, fullGeohash).
		WithContext(ctx).Exec()
	if err != nil {
		return nil, fmt.Errorf("failed to insert into posts_by_geohash: %w", err)
	}

	// Insert into posts_by_id
	err = r.session.Query(`
		INSERT INTO posts_by_id (post_id, user_id, content, media_urls, latitude, longitude, geohash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, postID, userID, req.Content, req.MediaURLs, req.Latitude, req.Longitude, fullGeohash, now).
		WithContext(ctx).Exec()
	if err != nil {
		return nil, fmt.Errorf("failed to insert into posts_by_id: %w", err)
	}

	// Insert into posts_by_user
	err = r.session.Query(`
		INSERT INTO posts_by_user (user_id, created_at, post_id, content, media_urls, latitude, longitude)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, userID, now, postID, req.Content, req.MediaURLs, req.Latitude, req.Longitude).
		WithContext(ctx).Exec()
	if err != nil {
		return nil, fmt.Errorf("failed to insert into posts_by_user: %w", err)
	}

	return &Post{
		ID:        postID.String(),
		UserID:    userID.String(),
		Content:   req.Content,
		MediaURLs: req.MediaURLs,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
		Geohash:   fullGeohash,
		CreatedAt: now,
	}, nil
}

// GetNearbyPosts retrieves posts sorted by proximity to a given location
func (r *PostRepository) GetNearbyPosts(ctx context.Context, latitude, longitude, radiusKM float64, limit int) ([]Post, error) {
	// Default values
	if radiusKM <= 0 {
		radiusKM = 10 // Default 10km radius
	}
	if limit <= 0 || limit > 100 {
		limit = 50 // Default 50 posts, max 100
	}

	// Get all 9 geohash cells to query
	neighbors := GetNeighbors(latitude, longitude)

	// Collect posts from all cells
	var allPosts []Post

	for _, geohashPrefix := range neighbors {
		iter := r.session.Query(`
			SELECT post_id, user_id, content, media_urls, latitude, longitude, full_geohash, created_at
			FROM posts_by_geohash
			WHERE geohash_prefix = ?
			LIMIT ?
		`, geohashPrefix, limit*2). // Fetch more to account for distance filtering
			WithContext(ctx).Iter()

		var post Post
		var postID, userID gocql.UUID
		var mediaURLs []string

		for iter.Scan(&postID, &userID, &post.Content, &mediaURLs, &post.Latitude, &post.Longitude, &post.Geohash, &post.CreatedAt) {
			post.ID = postID.String()
			post.UserID = userID.String()
			post.MediaURLs = mediaURLs

			// Calculate distance and filter
			distance := HaversineDistance(latitude, longitude, post.Latitude, post.Longitude)
			if distance <= radiusKM {
				post.Distance = distance
				allPosts = append(allPosts, post)
			}

			// Reset for next iteration
			post = Post{}
			mediaURLs = nil
		}

		if err := iter.Close(); err != nil {
			return nil, fmt.Errorf("error iterating posts: %w", err)
		}
	}

	// Sort by distance
	sort.Slice(allPosts, func(i, j int) bool {
		return allPosts[i].Distance < allPosts[j].Distance
	})

	// Apply limit
	if len(allPosts) > limit {
		allPosts = allPosts[:limit]
	}

	return allPosts, nil
}

// GetPostByID retrieves a post by its ID
func (r *PostRepository) GetPostByID(ctx context.Context, id string) (*Post, error) {
	postID, err := gocql.ParseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("invalid post_id: %w", err)
	}

	var post Post
	var userID gocql.UUID
	var mediaURLs []string

	err = r.session.Query(`
		SELECT post_id, user_id, content, media_urls, latitude, longitude, geohash, created_at
		FROM posts_by_id
		WHERE post_id = ?
	`, postID).WithContext(ctx).Scan(&postID, &userID, &post.Content, &mediaURLs, &post.Latitude, &post.Longitude, &post.Geohash, &post.CreatedAt)

	if err != nil {
		if err == gocql.ErrNotFound {
			return nil, fmt.Errorf("post not found")
		}
		return nil, fmt.Errorf("failed to get post: %w", err)
	}

	post.ID = postID.String()
	post.UserID = userID.String()
	post.MediaURLs = mediaURLs

	return &post, nil
}

// GetPostsByUser retrieves all posts by a user
func (r *PostRepository) GetPostsByUser(ctx context.Context, userIDStr string, limit int) ([]Post, error) {
	userID, err := gocql.ParseUUID(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	iter := r.session.Query(`
		SELECT post_id, content, media_urls, latitude, longitude, created_at
		FROM posts_by_user
		WHERE user_id = ?
		LIMIT ?
	`, userID, limit).WithContext(ctx).Iter()

	var posts []Post
	var post Post
	var postID gocql.UUID
	var mediaURLs []string

	for iter.Scan(&postID, &post.Content, &mediaURLs, &post.Latitude, &post.Longitude, &post.CreatedAt) {
		post.ID = postID.String()
		post.UserID = userIDStr
		post.MediaURLs = mediaURLs
		posts = append(posts, post)

		// Reset for next iteration
		post = Post{}
		mediaURLs = nil
	}

	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("error iterating posts: %w", err)
	}

	return posts, nil
}
