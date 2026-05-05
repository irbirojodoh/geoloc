package search

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gocql/gocql"

	"social-geo-go/internal/data"
)

// HydratePosts takes a slice of post_ids and returns full Post objects
// by querying posts_by_id in Cassandra. Uses concurrent queries with a
// semaphore of max 10 in-flight requests. Preserves ordering of input IDs.
func HydratePosts(ctx context.Context, ids []string, session *gocql.Session) ([]data.Post, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	type result struct {
		index int
		post  *data.Post
		err   error
	}

	sem := make(chan struct{}, 10)
	results := make(chan result, len(ids))
	var wg sync.WaitGroup

	for i, id := range ids {
		wg.Add(1)
		go func(idx int, postID string) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			post, err := getPostByID(ctx, session, postID)
			results <- result{idx, post, err}
		}(i, id)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	ordered := make([]*data.Post, len(ids))
	var firstErr error
	for r := range results {
		if r.err != nil {
			slog.Warn("failed to hydrate post", "post_id", ids[r.index], "error", r.err)
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		ordered[r.index] = r.post
	}

	// Filter out nil entries (failed hydrations)
	posts := make([]data.Post, 0, len(ids))
	for _, p := range ordered {
		if p != nil {
			posts = append(posts, *p)
		}
	}

	if len(posts) == 0 && firstErr != nil {
		return nil, fmt.Errorf("hydrate posts: all queries failed, last error: %w", firstErr)
	}

	return posts, nil
}

// HydrateUsers takes a slice of user_ids and returns full User objects
// by querying the users table. Same concurrency pattern as HydratePosts.
func HydrateUsers(ctx context.Context, ids []string, session *gocql.Session) ([]data.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	type result struct {
		index int
		user  *data.User
		err   error
	}

	sem := make(chan struct{}, 10)
	results := make(chan result, len(ids))
	var wg sync.WaitGroup

	for i, id := range ids {
		wg.Add(1)
		go func(idx int, userID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			user, err := getUserByID(ctx, session, userID)
			results <- result{idx, user, err}
		}(i, id)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	ordered := make([]*data.User, len(ids))
	var firstErr error
	for r := range results {
		if r.err != nil {
			slog.Warn("failed to hydrate user", "user_id", ids[r.index], "error", r.err)
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		ordered[r.index] = r.user
	}

	users := make([]data.User, 0, len(ids))
	for _, u := range ordered {
		if u != nil {
			users = append(users, *u)
		}
	}

	if len(users) == 0 && firstErr != nil {
		return nil, fmt.Errorf("hydrate users: all queries failed, last error: %w", firstErr)
	}

	return users, nil
}

// getPostByID fetches a single post from Cassandra by its UUID string.
func getPostByID(ctx context.Context, session *gocql.Session, id string) (*data.Post, error) {
	postID, err := gocql.ParseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("invalid post_id: %w", err)
	}

	var post data.Post
	var userID gocql.UUID
	var mediaURLs []string

	err = session.Query(`
		SELECT post_id, user_id, content, media_urls, latitude, longitude, geohash, created_at
		FROM posts_by_id
		WHERE post_id = ?
	`, postID).WithContext(ctx).Scan(
		&postID, &userID, &post.Content, &mediaURLs,
		&post.Latitude, &post.Longitude, &post.Geohash, &post.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	post.ID = postID.String()
	post.UserID = userID.String()
	post.MediaURLs = mediaURLs

	return &post, nil
}

// getUserByID fetches a single user from Cassandra by its UUID string.
func getUserByID(ctx context.Context, session *gocql.Session, id string) (*data.User, error) {
	userID, err := gocql.ParseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	var user data.User
	err = session.Query(`
		SELECT id, username, email, full_name, bio, phone_number, profile_picture_url, password_hash, is_deleted, created_at, updated_at
		FROM users
		WHERE id = ?
	`, userID).WithContext(ctx).Scan(
		&userID, &user.Username, &user.Email, &user.FullName,
		&user.Bio, &user.PhoneNumber, &user.ProfilePictureURL, &user.PasswordHash,
		&user.IsDeleted, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	user.ID = userID.String()
	if user.CoverImageURL == "" {
		user.CoverImageURL = data.DefaultCoverImageURL
	}

	return &user, nil
}

// Ensure unused import satisfies compiler (time imported for potential future use).
var _ = time.Now
