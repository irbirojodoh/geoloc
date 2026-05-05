package data

import (
	"context"
	"fmt"
	"sort"
	"strings"
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

	batch := r.session.NewBatch(gocql.LoggedBatch)
	batch.WithContext(ctx)

	// Insert into posts_by_geohash
	batch.Query(`
		INSERT INTO posts_by_geohash (geohash_prefix, created_at, post_id, user_id, content, media_urls, latitude, longitude, full_geohash, ip_address, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, geohashPrefix, now, postID, userID, req.Content, req.MediaURLs, req.Latitude, req.Longitude, fullGeohash, req.IPAddress, req.UserAgent)

	// Insert into posts_by_id
	batch.Query(`
		INSERT INTO posts_by_id (post_id, user_id, content, media_urls, latitude, longitude, geohash, ip_address, user_agent, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, postID, userID, req.Content, req.MediaURLs, req.Latitude, req.Longitude, fullGeohash, req.IPAddress, req.UserAgent, now)

	// Insert into posts_by_user
	batch.Query(`
		INSERT INTO posts_by_user (user_id, created_at, post_id, content, media_urls, latitude, longitude, ip_address, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, now, postID, req.Content, req.MediaURLs, req.Latitude, req.Longitude, req.IPAddress, req.UserAgent)

	err = r.session.ExecuteBatch(batch)
	if err != nil {
		return nil, fmt.Errorf("failed to create post: %w", err)
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
// cursorTime is used for pagination - only returns posts created before this time
func (r *PostRepository) GetNearbyPosts(ctx context.Context, latitude, longitude, radiusKM float64, limit int, cursorTime time.Time) ([]Post, error) {
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
		var iter *gocql.Iter

		if cursorTime.IsZero() {
			// No cursor - get newest posts
			iter = r.session.Query(`
				SELECT post_id, user_id, content, media_urls, latitude, longitude, full_geohash, created_at
				FROM posts_by_geohash
				WHERE geohash_prefix = ?
				ORDER BY created_at DESC
				LIMIT ?
			`, geohashPrefix, limit*2).WithContext(ctx).Iter()
		} else {
			// With cursor - get posts older than cursor
			iter = r.session.Query(`
				SELECT post_id, user_id, content, media_urls, latitude, longitude, full_geohash, created_at
				FROM posts_by_geohash
				WHERE geohash_prefix = ? AND created_at < ?
				ORDER BY created_at DESC
				LIMIT ?
			`, geohashPrefix, cursorTime, limit*2).WithContext(ctx).Iter()
		}

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

	// Sort by created_at (newest first) for consistent pagination
	sort.Slice(allPosts, func(i, j int) bool {
		return allPosts[i].CreatedAt.After(allPosts[j].CreatedAt)
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

// GetPostsByUser retrieves all posts by a user with cursor-based pagination
func (r *PostRepository) GetPostsByUser(ctx context.Context, userIDStr string, limit int, cursorTime time.Time) ([]Post, error) {
	userID, err := gocql.ParseUUID(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var iter *gocql.Iter

	if cursorTime.IsZero() {
		// No cursor - get newest posts
		iter = r.session.Query(`
			SELECT post_id, content, media_urls, latitude, longitude, created_at
			FROM posts_by_user
			WHERE user_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		`, userID, limit).WithContext(ctx).Iter()
	} else {
		// With cursor - get posts older than cursor
		iter = r.session.Query(`
			SELECT post_id, content, media_urls, latitude, longitude, created_at
			FROM posts_by_user
			WHERE user_id = ? AND created_at < ?
			ORDER BY created_at DESC
			LIMIT ?
		`, userID, cursorTime, limit).WithContext(ctx).Iter()
	}

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

// SearchPosts searches for posts by content using SAI index.
// Uses LIKE prefix matching on the content SAI index instead of full table scan.
func (r *PostRepository) SearchPosts(ctx context.Context, query string, limit int) ([]Post, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	if normalizedQuery == "" {
		return nil, nil
	}

	searchPattern := "%" + normalizedQuery + "%"

	iter := r.session.Query(`
		SELECT post_id, user_id, content, media_urls, latitude, longitude, geohash, created_at
		FROM posts_by_id
		WHERE content LIKE ?
		LIMIT ?
	`, searchPattern, limit).WithContext(ctx).Iter()

	var posts []Post
	var post Post
	var postID, userID gocql.UUID
	var mediaURLs []string

	for iter.Scan(&postID, &userID, &post.Content, &mediaURLs,
		&post.Latitude, &post.Longitude, &post.Geohash, &post.CreatedAt) {
		post.ID = postID.String()
		post.UserID = userID.String()
		post.MediaURLs = mediaURLs
		posts = append(posts, post)

		post = Post{}
		mediaURLs = nil
		if len(posts) >= limit {
			break
		}
	}

	if err := iter.Close(); err != nil {
		if len(posts) == 0 {
			return r.searchPostsByScan(ctx, normalizedQuery, limit)
		}
		return posts, nil
	}

	if len(posts) > 0 {
		return posts, nil
	}

	return r.searchPostsByScan(ctx, normalizedQuery, limit)
}

func (r *PostRepository) searchPostsByScan(ctx context.Context, query string, limit int) ([]Post, error) {
	scanLimit := limit * 20
	if scanLimit < 100 {
		scanLimit = 100
	}
	if scanLimit > 1000 {
		scanLimit = 1000
	}

	iter := r.session.Query(`
		SELECT post_id, user_id, content, media_urls, latitude, longitude, geohash, created_at
		FROM posts_by_id
		LIMIT ?
	`, scanLimit).WithContext(ctx).Iter()

	var posts []Post
	var post Post
	var postID, userID gocql.UUID
	var mediaURLs []string

	for iter.Scan(&postID, &userID, &post.Content, &mediaURLs,
		&post.Latitude, &post.Longitude, &post.Geohash, &post.CreatedAt) {
		if strings.Contains(strings.ToLower(post.Content), query) {
			post.ID = postID.String()
			post.UserID = userID.String()
			post.MediaURLs = mediaURLs
			posts = append(posts, post)
		}

		post = Post{}
		mediaURLs = nil
		if len(posts) >= limit {
			break
		}
	}

	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("search posts fallback scan failed: %w", err)
	}

	return posts, nil
}

// DeletePost removes a post from all denormalized tables.
// Verifies ownership: only the post author can delete their post.
func (r *PostRepository) DeletePost(ctx context.Context, postIDStr, requestingUserID string) error {
	postID, err := gocql.ParseUUID(postIDStr)
	if err != nil {
		return fmt.Errorf("invalid post_id: %w", err)
	}

	// Fetch the post to get the data needed for multi-table deletion
	var userID gocql.UUID
	var latitude, longitude float64
	var createdAt time.Time
	var geohash string

	err = r.session.Query(`
		SELECT user_id, latitude, longitude, geohash, created_at
		FROM posts_by_id WHERE post_id = ?
	`, postID).WithContext(ctx).Scan(&userID, &latitude, &longitude, &geohash, &createdAt)

	if err != nil {
		if err == gocql.ErrNotFound {
			return fmt.Errorf("post not found")
		}
		return fmt.Errorf("failed to fetch post: %w", err)
	}

	// Verify ownership
	if userID.String() != requestingUserID {
		return fmt.Errorf("forbidden: you can only delete your own posts")
	}

	// Calculate geohash prefix for posts_by_geohash deletion
	geohashPrefix := GetGeohashPrefix(latitude, longitude)

	// Batch delete from all denormalized tables
	// Note: Counter tables (like_counts, comment_counts) cannot be part of a logged batch
	batch := r.session.NewBatch(gocql.LoggedBatch).WithContext(ctx)

	// Delete from posts_by_id
	batch.Query(`DELETE FROM posts_by_id WHERE post_id = ?`, postID)

	// Delete from posts_by_geohash
	batch.Query(`DELETE FROM posts_by_geohash WHERE geohash_prefix = ? AND created_at = ? AND post_id = ?`,
		geohashPrefix, createdAt, postID)

	// Delete from posts_by_user
	batch.Query(`DELETE FROM posts_by_user WHERE user_id = ? AND created_at = ? AND post_id = ?`,
		userID, createdAt, postID)

	// Delete associated likes
	batch.Query(`DELETE FROM likes WHERE target_type = ? AND target_id = ?`, TargetTypePost, postID)
	batch.Query(`DELETE FROM like_state WHERE target_type = ? AND target_id = ?`, TargetTypePost, postID)

	// Delete associated comments
	batch.Query(`DELETE FROM comments WHERE post_id = ?`, postID)

	err = r.session.ExecuteBatch(batch)
	if err != nil {
		return fmt.Errorf("failed to delete post: %w", err)
	}

	// Delete counter tables separately (cannot be in a logged batch)
	_ = r.session.Query(`DELETE FROM like_counts WHERE target_type = ? AND target_id = ?`,
		TargetTypePost, postID).WithContext(ctx).Exec()
	_ = r.session.Query(`DELETE FROM comment_counts WHERE post_id = ?`,
		postID).WithContext(ctx).Exec()

	return nil
}
