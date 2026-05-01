package cache

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// CommentCounter handles Redis-based comment counting operations
type CommentCounter struct {
	client *redis.Client
}

// NewCommentCounter creates a new CommentCounter with the given Redis client
func NewCommentCounter(redisClient *RedisClient) *CommentCounter {
	return &CommentCounter{client: redisClient.Client()}
}

// commentCountKey generates the Redis key for a comment count
func commentCountKey(postID string) string {
	return fmt.Sprintf("comment_count:%s", postID)
}

// IncrementCommentCount atomically increments the comment count for a post
func (cc *CommentCounter) IncrementCommentCount(ctx context.Context, postID string) (int64, error) {
	key := commentCountKey(postID)
	count, err := cc.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to increment comment count: %w", err)
	}
	return count, nil
}

// DecrementCommentCount atomically decrements the comment count for a post
// Returns the new count after decrement (minimum 0)
func (cc *CommentCounter) DecrementCommentCount(ctx context.Context, postID string) (int64, error) {
	key := commentCountKey(postID)

	// Use Lua script to ensure count doesn't go below 0
	script := redis.NewScript(`
		local current = redis.call('GET', KEYS[1])
		if current == false or tonumber(current) <= 0 then
			redis.call('SET', KEYS[1], 0)
			return 0
		end
		return redis.call('DECR', KEYS[1])
	`)

	result, err := script.Run(ctx, cc.client, []string{key}).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to decrement comment count: %w", err)
	}

	count, ok := result.(int64)
	if !ok {
		return 0, nil
	}
	return count, nil
}

// GetCommentCount retrieves the comment count for a post
func (cc *CommentCounter) GetCommentCount(ctx context.Context, postID string) (int64, error) {
	key := commentCountKey(postID)
	countStr, err := cc.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, nil // Key doesn't exist, count is 0
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get comment count: %w", err)
	}

	count, err := strconv.ParseInt(countStr, 10, 64)
	if err != nil {
		return 0, nil
	}
	return count, nil
}

// GetCommentCountsBatch retrieves comment counts for multiple posts in a single round-trip
func (cc *CommentCounter) GetCommentCountsBatch(ctx context.Context, postIDs []string) (map[string]int64, error) {
	if len(postIDs) == 0 {
		return make(map[string]int64), nil
	}

	// Build keys
	keys := make([]string, len(postIDs))
	for i, id := range postIDs {
		keys[i] = commentCountKey(id)
	}

	// Use MGET for batch retrieval
	results, err := cc.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to batch get comment counts: %w", err)
	}

	counts := make(map[string]int64)
	for i, result := range results {
		if result == nil {
			counts[postIDs[i]] = 0
			continue
		}

		countStr, ok := result.(string)
		if !ok {
			counts[postIDs[i]] = 0
			continue
		}

		count, err := strconv.ParseInt(countStr, 10, 64)
		if err != nil {
			counts[postIDs[i]] = 0
			continue
		}
		counts[postIDs[i]] = count
	}

	return counts, nil
}

// SetCommentCount sets the comment count for a post (useful for initialization/recovery)
func (cc *CommentCounter) SetCommentCount(ctx context.Context, postID string, count int64) error {
	key := commentCountKey(postID)
	return cc.client.Set(ctx, key, count, 0).Err()
}

// DeleteCommentCount removes the comment count key (for cleanup/testing)
func (cc *CommentCounter) DeleteCommentCount(ctx context.Context, postID string) error {
	key := commentCountKey(postID)
	return cc.client.Del(ctx, key).Err()
}
