package cache

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// LikeCounter handles Redis-based like counting operations
type LikeCounter struct {
	client *redis.Client
}

// NewLikeCounter creates a new LikeCounter with the given Redis client
func NewLikeCounter(redisClient *RedisClient) *LikeCounter {
	return &LikeCounter{client: redisClient.Client()}
}

// likeCountKey generates the Redis key for a like count
func likeCountKey(targetType, targetID string) string {
	return fmt.Sprintf("like_count:%s:%s", targetType, targetID)
}

// IncrementLikeCount atomically increments the like count for a target
// Returns the new count after increment
func (lc *LikeCounter) IncrementLikeCount(ctx context.Context, targetType, targetID string) (int64, error) {
	key := likeCountKey(targetType, targetID)
	count, err := lc.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to increment like count: %w", err)
	}
	return count, nil
}

// DecrementLikeCount atomically decrements the like count for a target
// Returns the new count after decrement (minimum 0)
func (lc *LikeCounter) DecrementLikeCount(ctx context.Context, targetType, targetID string) (int64, error) {
	key := likeCountKey(targetType, targetID)

	// Use Lua script to ensure count doesn't go below 0
	script := redis.NewScript(`
		local current = redis.call('GET', KEYS[1])
		if current == false or tonumber(current) <= 0 then
			redis.call('SET', KEYS[1], 0)
			return 0
		end
		return redis.call('DECR', KEYS[1])
	`)

	result, err := script.Run(ctx, lc.client, []string{key}).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to decrement like count: %w", err)
	}

	count, ok := result.(int64)
	if !ok {
		return 0, nil
	}
	return count, nil
}

// GetLikeCount retrieves the like count for a target
func (lc *LikeCounter) GetLikeCount(ctx context.Context, targetType, targetID string) (int64, error) {
	key := likeCountKey(targetType, targetID)
	countStr, err := lc.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, nil // Key doesn't exist, count is 0
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get like count: %w", err)
	}

	count, err := strconv.ParseInt(countStr, 10, 64)
	if err != nil {
		return 0, nil
	}
	return count, nil
}

// GetLikeCountsBatch retrieves like counts for multiple targets in a single round-trip
func (lc *LikeCounter) GetLikeCountsBatch(ctx context.Context, targetType string, targetIDs []string) (map[string]int64, error) {
	if len(targetIDs) == 0 {
		return make(map[string]int64), nil
	}

	// Build keys
	keys := make([]string, len(targetIDs))
	for i, id := range targetIDs {
		keys[i] = likeCountKey(targetType, id)
	}

	// Use MGET for batch retrieval
	results, err := lc.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to batch get like counts: %w", err)
	}

	counts := make(map[string]int64)
	for i, result := range results {
		if result == nil {
			counts[targetIDs[i]] = 0
			continue
		}

		countStr, ok := result.(string)
		if !ok {
			counts[targetIDs[i]] = 0
			continue
		}

		count, err := strconv.ParseInt(countStr, 10, 64)
		if err != nil {
			counts[targetIDs[i]] = 0
			continue
		}
		counts[targetIDs[i]] = count
	}

	return counts, nil
}

// SetLikeCount sets the like count for a target (useful for initialization/recovery)
func (lc *LikeCounter) SetLikeCount(ctx context.Context, targetType, targetID string, count int64) error {
	key := likeCountKey(targetType, targetID)
	return lc.client.Set(ctx, key, count, 0).Err()
}

// DeleteLikeCount removes the like count key (for cleanup/testing)
func (lc *LikeCounter) DeleteLikeCount(ctx context.Context, targetType, targetID string) error {
	key := likeCountKey(targetType, targetID)
	return lc.client.Del(ctx, key).Err()
}
