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

	batch := r.session.NewBatch(gocql.LoggedBatch).WithContext(ctx)

	// Add to follows table
	batch.Query(`
		INSERT INTO follows (follower_id, following_id, created_at)
		VALUES (?, ?, ?)
	`, fid, fgid, now)

	// Add to followers table (reverse)
	batch.Query(`
		INSERT INTO followers (user_id, follower_id, created_at)
		VALUES (?, ?, ?)
	`, fgid, fid, now)

	err = r.session.ExecuteBatch(batch)
	if err != nil {
		return fmt.Errorf("ERROR: failed to add follow: %w", err)
	}

	counterBatch := r.session.NewBatch(gocql.CounterBatch).WithContext(ctx)

	counterBatch.Query(`
        UPDATE follow_counts SET following_count = following_count + 1
        WHERE user_id = ?
    `, fid)

	counterBatch.Query(`
        UPDATE follow_counts SET followers_count = followers_count + 1
        WHERE user_id = ?
    `, fgid)

	if err := r.session.ExecuteBatch(counterBatch); err != nil {
		// In a real system, you might want to retry this or log it to a queue
		fmt.Printf("WARNING: failed to update follow counters: %v\n", err)
	}

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
		if err == gocql.ErrNotFound {
			return fmt.Errorf("follow relationship not found")
		}
		return fmt.Errorf("ERROR: follow relationship: %w", err)
	}

	batch := r.session.NewBatch(gocql.LoggedBatch).WithContext(ctx)

	// Delete from follows
	batch.Query(`
		DELETE FROM follows WHERE follower_id = ? AND following_id = ?
	`, fid, fgid)

	// Delete from followers
	batch.Query(`
		DELETE FROM followers WHERE user_id = ? AND created_at = ? AND follower_id = ?
	`, fgid, createdAt, fid)

	err = r.session.ExecuteBatch(batch)
	if err != nil {
		return fmt.Errorf("ERROR: unfolllow failed: %w", err)
	}

	counterBatch := r.session.NewBatch(gocql.CounterBatch).WithContext(ctx)
	counterBatch.Query(`UPDATE follow_counts SET following_count = following_count - 1 WHERE user_id = ?`, fid)
	counterBatch.Query(`UPDATE follow_counts SET followers_count = followers_count - 1 WHERE user_id = ?`, fgid)

	if err := r.session.ExecuteBatch(counterBatch); err != nil {
		fmt.Printf("WARNING: failed to update unfollow counters: %v\n", err)
	}

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
