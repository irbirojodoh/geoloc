package data

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"
)

type LikeRepository struct {
	session *gocql.Session
}

func NewLikeRepository(session *gocql.Session) *LikeRepository {
	return &LikeRepository{session: session}
}

// AddLike adds a like to a post or comment
func (r *LikeRepository) AddLike(ctx context.Context, req *LikeRequest) (*Like, error) {
	targetID, err := gocql.ParseUUID(req.TargetID)
	if err != nil {
		return nil, fmt.Errorf("invalid target_id: %w", err)
	}

	userID, err := gocql.ParseUUID(req.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	now := time.Now()

	// Insert like
	err = r.session.Query(`
		INSERT INTO likes (target_type, target_id, user_id, created_at)
		VALUES (?, ?, ?, ?)
	`, req.TargetType, targetID, userID, now).WithContext(ctx).Exec()
	if err != nil {
		return nil, fmt.Errorf("failed to add like: %w", err)
	}

	// Increment counter
	err = r.session.Query(`
		UPDATE like_counts SET count = count + 1
		WHERE target_type = ? AND target_id = ?
	`, req.TargetType, targetID).WithContext(ctx).Exec()
	if err != nil {
		// Log but don't fail - counter is denormalized
		fmt.Printf("Warning: failed to update like count: %v\n", err)
	}

	return &Like{
		TargetType: req.TargetType,
		TargetID:   targetID.String(),
		UserID:     userID.String(),
		CreatedAt:  now,
	}, nil
}

// RemoveLike removes a like from a post or comment
func (r *LikeRepository) RemoveLike(ctx context.Context, req *LikeRequest) error {
	targetID, err := gocql.ParseUUID(req.TargetID)
	if err != nil {
		return fmt.Errorf("invalid target_id: %w", err)
	}

	userID, err := gocql.ParseUUID(req.UserID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	// Delete like
	err = r.session.Query(`
		DELETE FROM likes
		WHERE target_type = ? AND target_id = ? AND user_id = ?
	`, req.TargetType, targetID, userID).WithContext(ctx).Exec()
	if err != nil {
		return fmt.Errorf("failed to remove like: %w", err)
	}

	// Decrement counter
	err = r.session.Query(`
		UPDATE like_counts SET count = count - 1
		WHERE target_type = ? AND target_id = ?
	`, req.TargetType, targetID).WithContext(ctx).Exec()
	if err != nil {
		fmt.Printf("Warning: failed to update like count: %v\n", err)
	}

	return nil
}

// GetLikeCount returns the like count for a target
func (r *LikeRepository) GetLikeCount(ctx context.Context, targetType, targetID string) (int64, error) {
	tid, err := gocql.ParseUUID(targetID)
	if err != nil {
		return 0, fmt.Errorf("invalid target_id: %w", err)
	}

	var count int64
	err = r.session.Query(`
		SELECT count FROM like_counts
		WHERE target_type = ? AND target_id = ?
	`, targetType, tid).WithContext(ctx).Scan(&count)
	if err != nil {
		if err == gocql.ErrNotFound {
			return 0, nil
		}
		return 0, err
	}

	return count, nil
}

// HasUserLiked checks if a user has liked a target
func (r *LikeRepository) HasUserLiked(ctx context.Context, targetType, targetID, userID string) (bool, error) {
	tid, err := gocql.ParseUUID(targetID)
	if err != nil {
		return false, nil
	}

	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return false, nil
	}

	var count int
	err = r.session.Query(`
		SELECT COUNT(*) FROM likes
		WHERE target_type = ? AND target_id = ? AND user_id = ?
	`, targetType, tid, uid).WithContext(ctx).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}
