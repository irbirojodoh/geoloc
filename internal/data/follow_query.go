package data

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"
)

type FollowRepository struct {
	session *gocql.Session
}

func NewFollowRepository(session *gocql.Session) *FollowRepository {
	return &FollowRepository{session: session}
}

// Follow adds a follow relationship
func (r *FollowRepository) Follow(ctx context.Context, followerID, followingID string) error {
	fid, err := gocql.ParseUUID(followerID)
	if err != nil {
		return fmt.Errorf("invalid follower_id: %w", err)
	}

	fgid, err := gocql.ParseUUID(followingID)
	if err != nil {
		return fmt.Errorf("invalid following_id: %w", err)
	}

	if followerID == followingID {
		return fmt.Errorf("cannot follow yourself")
	}

	now := time.Now()

	// Add to follows table
	err = r.session.Query(`
		INSERT INTO follows (follower_id, following_id, created_at)
		VALUES (?, ?, ?)
	`, fid, fgid, now).WithContext(ctx).Exec()
	if err != nil {
		return fmt.Errorf("failed to add follow: %w", err)
	}

	// Add to followers table (reverse)
	err = r.session.Query(`
		INSERT INTO followers (user_id, follower_id, created_at)
		VALUES (?, ?, ?)
	`, fgid, fid, now).WithContext(ctx).Exec()
	if err != nil {
		return fmt.Errorf("failed to add follower: %w", err)
	}

	// Update counters
	r.session.Query(`
		UPDATE follow_counts SET following_count = following_count + 1
		WHERE user_id = ?
	`, fid).WithContext(ctx).Exec()

	r.session.Query(`
		UPDATE follow_counts SET followers_count = followers_count + 1
		WHERE user_id = ?
	`, fgid).WithContext(ctx).Exec()

	return nil
}

// Unfollow removes a follow relationship
func (r *FollowRepository) Unfollow(ctx context.Context, followerID, followingID string) error {
	fid, err := gocql.ParseUUID(followerID)
	if err != nil {
		return fmt.Errorf("invalid follower_id: %w", err)
	}

	fgid, err := gocql.ParseUUID(followingID)
	if err != nil {
		return fmt.Errorf("invalid following_id: %w", err)
	}

	// Get created_at for followers table deletion
	var createdAt time.Time
	err = r.session.Query(`
		SELECT created_at FROM follows WHERE follower_id = ? AND following_id = ?
	`, fid, fgid).WithContext(ctx).Scan(&createdAt)
	if err != nil {
		return fmt.Errorf("follow relationship not found")
	}

	// Delete from follows
	r.session.Query(`
		DELETE FROM follows WHERE follower_id = ? AND following_id = ?
	`, fid, fgid).WithContext(ctx).Exec()

	// Delete from followers
	r.session.Query(`
		DELETE FROM followers WHERE user_id = ? AND created_at = ? AND follower_id = ?
	`, fgid, createdAt, fid).WithContext(ctx).Exec()

	// Update counters
	r.session.Query(`
		UPDATE follow_counts SET following_count = following_count - 1
		WHERE user_id = ?
	`, fid).WithContext(ctx).Exec()

	r.session.Query(`
		UPDATE follow_counts SET followers_count = followers_count - 1
		WHERE user_id = ?
	`, fgid).WithContext(ctx).Exec()

	return nil
}

// IsFollowing checks if a user is following another
func (r *FollowRepository) IsFollowing(ctx context.Context, followerID, followingID string) (bool, error) {
	fid, err := gocql.ParseUUID(followerID)
	if err != nil {
		return false, nil
	}

	fgid, err := gocql.ParseUUID(followingID)
	if err != nil {
		return false, nil
	}

	var count int
	err = r.session.Query(`
		SELECT COUNT(*) FROM follows WHERE follower_id = ? AND following_id = ?
	`, fid, fgid).WithContext(ctx).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetFollowers returns users following a user
func (r *FollowRepository) GetFollowers(ctx context.Context, userID string, limit int) ([]string, error) {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	iter := r.session.Query(`
		SELECT follower_id FROM followers WHERE user_id = ? LIMIT ?
	`, uid, limit).WithContext(ctx).Iter()

	var followers []string
	var followerID gocql.UUID

	for iter.Scan(&followerID) {
		followers = append(followers, followerID.String())
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	return followers, nil
}

// GetFollowing returns users a user is following
func (r *FollowRepository) GetFollowing(ctx context.Context, userID string, limit int) ([]string, error) {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	iter := r.session.Query(`
		SELECT following_id FROM follows WHERE follower_id = ? LIMIT ?
	`, uid, limit).WithContext(ctx).Iter()

	var following []string
	var followingID gocql.UUID

	for iter.Scan(&followingID) {
		following = append(following, followingID.String())
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	return following, nil
}

// GetFollowCounts returns follower and following counts
func (r *FollowRepository) GetFollowCounts(ctx context.Context, userID string) (*FollowCounts, error) {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	var followersCount, followingCount int64
	err = r.session.Query(`
		SELECT followers_count, following_count FROM follow_counts WHERE user_id = ?
	`, uid).WithContext(ctx).Scan(&followersCount, &followingCount)
	if err != nil && err != gocql.ErrNotFound {
		return nil, err
	}

	return &FollowCounts{
		UserID:         userID,
		FollowersCount: followersCount,
		FollowingCount: followingCount,
	}, nil
}
