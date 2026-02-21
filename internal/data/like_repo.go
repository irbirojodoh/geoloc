package data

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"

	"social-geo-go/internal/cache"
)

// LikeRepository handles like operations with Cassandra (state) and Redis (counters)
type LikeRepository struct {
	session     *gocql.Session
	likeCounter *cache.LikeCounter
}

// NewLikeRepository creates a new LikeRepository
// likeCounter can be nil if Redis is not available (will fall back to Cassandra counters)
func NewLikeRepository(session *gocql.Session, likeCounter *cache.LikeCounter) *LikeRepository {
	return &LikeRepository{
		session:     session,
		likeCounter: likeCounter,
	}
}

// ToggleLikeResult represents the result of a toggle operation
type ToggleLikeResult struct {
	IsLiked   bool  `json:"is_liked"`
	LikeCount int64 `json:"like_count"`
	Changed   bool  `json:"changed"` // Whether the state actually changed
}

// ToggleLike toggles the like state for a user on a target (post or comment)
// This is idempotent - calling it twice with the same desired state will not double-count
// Uses Cassandra LWT for atomic state changes, Redis for counter updates
func (r *LikeRepository) ToggleLike(ctx context.Context, targetType, targetID, userID string, wantLiked bool) (*ToggleLikeResult, error) {
	tid, err := gocql.ParseUUID(targetID)
	if err != nil {
		return nil, fmt.Errorf("invalid target_id: %w", err)
	}

	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	now := time.Now()

	if wantLiked {
		// Try to add like using LWT (IF NOT EXISTS)
		return r.tryAddLike(ctx, targetType, tid, uid, now)
	} else {
		// Try to remove like using LWT (IF EXISTS)
		return r.tryRemoveLike(ctx, targetType, tid, uid)
	}
}

// tryAddLike attempts to add a like using LWT for idempotency
func (r *LikeRepository) tryAddLike(ctx context.Context, targetType string, targetID, userID gocql.UUID, now time.Time) (*ToggleLikeResult, error) {
	// Use LWT: INSERT ... IF NOT EXISTS
	// This returns applied=true only if the row didn't exist
	// LWT queries return a map with [applied] column
	resultMap := make(map[string]interface{})
	applied, err := r.session.Query(`
		INSERT INTO like_state (target_type, target_id, user_id, created_at)
		VALUES (?, ?, ?, ?)
		IF NOT EXISTS
	`, targetType, targetID, userID, now).
		WithContext(ctx).
		MapScanCAS(resultMap)

	if err != nil {
		return nil, fmt.Errorf("failed to add like: %w", err)
	}

	result := &ToggleLikeResult{
		IsLiked: true,
		Changed: applied, // applied=true means the insert succeeded (row didn't exist before)
	}

	// Only increment counter if state actually changed
	if applied && r.likeCounter != nil {
		count, err := r.likeCounter.IncrementLikeCount(ctx, targetType, targetID.String())
		if err != nil {
			// Log but don't fail - state change succeeded
			fmt.Printf("WARNING: failed to increment Redis counter: %v\n", err)
			// Fall back to getting estimate
			count, _ = r.getLikeCountFallback(ctx, targetType, targetID.String())
		}
		result.LikeCount = count
	} else if r.likeCounter != nil {
		// State didn't change, just get current count
		count, _ := r.likeCounter.GetLikeCount(ctx, targetType, targetID.String())
		result.LikeCount = count
	}

	// Also maintain the legacy likes table for backward compatibility
	if applied {
		go r.insertLegacyLike(context.Background(), targetType, targetID, userID, now)
	}

	return result, nil
}

// tryRemoveLike attempts to remove a like using LWT for idempotency
func (r *LikeRepository) tryRemoveLike(ctx context.Context, targetType string, targetID, userID gocql.UUID) (*ToggleLikeResult, error) {
	// First check if the like exists (required for conditional delete)
	var existingTime time.Time
	err := r.session.Query(`
		SELECT created_at FROM like_state
		WHERE target_type = ? AND target_id = ? AND user_id = ?
	`, targetType, targetID, userID).
		WithContext(ctx).
		Consistency(gocql.One).
		Scan(&existingTime)

	if err == gocql.ErrNotFound {
		// Already not liked - return current state without change
		result := &ToggleLikeResult{
			IsLiked: false,
			Changed: false,
		}
		if r.likeCounter != nil {
			result.LikeCount, _ = r.likeCounter.GetLikeCount(ctx, targetType, targetID.String())
		}
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to check like state: %w", err)
	}

	// Like exists, delete it using LWT
	// Use MapScanCAS for LWT queries
	resultMap := make(map[string]interface{})
	applied, err := r.session.Query(`
		DELETE FROM like_state
		WHERE target_type = ? AND target_id = ? AND user_id = ?
		IF EXISTS
	`, targetType, targetID, userID).
		WithContext(ctx).
		MapScanCAS(resultMap)

	if err != nil {
		return nil, fmt.Errorf("failed to remove like: %w", err)
	}

	result := &ToggleLikeResult{
		IsLiked: false,
		Changed: applied,
	}

	// Only decrement counter if state actually changed
	if applied && r.likeCounter != nil {
		count, err := r.likeCounter.DecrementLikeCount(ctx, targetType, targetID.String())
		if err != nil {
			fmt.Printf("WARNING: failed to decrement Redis counter: %v\n", err)
			count, _ = r.getLikeCountFallback(ctx, targetType, targetID.String())
		}
		result.LikeCount = count
	} else if r.likeCounter != nil {
		count, _ := r.likeCounter.GetLikeCount(ctx, targetType, targetID.String())
		result.LikeCount = count
	}

	// Also delete from legacy likes table
	if applied {
		go r.deleteLegacyLike(context.Background(), targetType, targetID, userID)
	}

	return result, nil
}

// insertLegacyLike maintains backward compatibility with the likes table
func (r *LikeRepository) insertLegacyLike(ctx context.Context, targetType string, targetID, userID gocql.UUID, createdAt time.Time) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_ = r.session.Query(`
		INSERT INTO likes (target_type, target_id, user_id, created_at)
		VALUES (?, ?, ?, ?)
	`, targetType, targetID, userID, createdAt).WithContext(ctx).Exec()
}

// deleteLegacyLike removes from the legacy likes table
func (r *LikeRepository) deleteLegacyLike(ctx context.Context, targetType string, targetID, userID gocql.UUID) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_ = r.session.Query(`
		DELETE FROM likes WHERE target_type = ? AND target_id = ? AND user_id = ?
	`, targetType, targetID, userID).WithContext(ctx).Exec()
}

// getLikeCountFallback counts likes from Cassandra when Redis is unavailable
func (r *LikeRepository) getLikeCountFallback(ctx context.Context, targetType, targetID string) (int64, error) {
	tid, err := gocql.ParseUUID(targetID)
	if err != nil {
		return 0, err
	}

	var count int
	err = r.session.Query(`
		SELECT COUNT(*) FROM like_state
		WHERE target_type = ? AND target_id = ?
	`, targetType, tid).WithContext(ctx).Scan(&count)
	if err != nil {
		return 0, err
	}
	return int64(count), nil
}

// ============== READ OPERATIONS ==============

// GetLikeCount returns the like count for a target
// Uses Redis if available, falls back to Cassandra
func (r *LikeRepository) GetLikeCount(ctx context.Context, targetType, targetID string) (int64, error) {
	if r.likeCounter != nil {
		count, err := r.likeCounter.GetLikeCount(ctx, targetType, targetID)
		if err == nil {
			return count, nil
		}
		// Fall back to Cassandra on Redis error
		fmt.Printf("WARNING: Redis GetLikeCount failed, falling back to Cassandra: %v\n", err)
	}

	return r.getLikeCountFallback(ctx, targetType, targetID)
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

	var createdAt time.Time
	err = r.session.Query(`
		SELECT created_at FROM like_state
		WHERE target_type = ? AND target_id = ? AND user_id = ?
	`, targetType, tid, uid).WithContext(ctx).Consistency(gocql.One).Scan(&createdAt)

	if err == gocql.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// PostLikeInfo contains like information for a post
type PostLikeInfo struct {
	LikeCount int64
	IsLiked   bool
}

// GetLikesForPosts returns like counts and user like status for multiple posts
func (r *LikeRepository) GetLikesForPosts(ctx context.Context, postIDs []string, userID string) (map[string]PostLikeInfo, error) {
	result := make(map[string]PostLikeInfo)

	if len(postIDs) == 0 {
		return result, nil
	}

	// Batch get counts from Redis
	var counts map[string]int64
	if r.likeCounter != nil {
		var err error
		counts, err = r.likeCounter.GetLikeCountsBatch(ctx, TargetTypePost, postIDs)
		if err != nil {
			fmt.Printf("WARNING: Redis batch get failed: %v\n", err)
			counts = make(map[string]int64)
		}
	} else {
		counts = make(map[string]int64)
	}

	// Check each post's like status
	for _, postID := range postIDs {
		info := PostLikeInfo{
			LikeCount: counts[postID],
		}

		// If Redis count is 0, try Cassandra fallback
		if info.LikeCount == 0 && r.likeCounter == nil {
			count, _ := r.getLikeCountFallback(ctx, TargetTypePost, postID)
			info.LikeCount = count
		}

		// Check if user has liked
		if userID != "" {
			liked, _ := r.HasUserLiked(ctx, TargetTypePost, postID, userID)
			info.IsLiked = liked
		}

		result[postID] = info
	}

	return result, nil
}

// ============== LEGACY API (for backward compatibility) ==============

// AddLike adds a like (legacy API - wraps ToggleLike)
func (r *LikeRepository) AddLike(ctx context.Context, req *LikeRequest) (*Like, error) {
	result, err := r.ToggleLike(ctx, req.TargetType, req.TargetID, req.UserID, true)
	if err != nil {
		return nil, err
	}

	return &Like{
		TargetType: req.TargetType,
		TargetID:   req.TargetID,
		UserID:     req.UserID,
		CreatedAt:  time.Now(),
		LikeCount:  result.LikeCount,
	}, nil
}

// RemoveLike removes a like (legacy API - wraps ToggleLike)
func (r *LikeRepository) RemoveLike(ctx context.Context, req *LikeRequest) error {
	_, err := r.ToggleLike(ctx, req.TargetType, req.TargetID, req.UserID, false)
	return err
}

// SyncCounterFromCassandra rebuilds a Redis counter from Cassandra data
// Useful for recovery or initial sync
func (r *LikeRepository) SyncCounterFromCassandra(ctx context.Context, targetType, targetID string) error {
	if r.likeCounter == nil {
		return fmt.Errorf("Redis counter not available")
	}

	count, err := r.getLikeCountFallback(ctx, targetType, targetID)
	if err != nil {
		return err
	}

	return r.likeCounter.SetLikeCount(ctx, targetType, targetID, count)
}
