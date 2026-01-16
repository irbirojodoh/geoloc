package data

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLikeRepository_Integration(t *testing.T) {
	// Pass nil for likeCounter to test Cassandra-only mode
	repo := NewLikeRepository(testSession, nil)
	ctx := context.Background()

	// Mock IDs
	postID := uuid.New().String()
	userID := uuid.New().String()

	t.Run("Add Like", func(t *testing.T) {
		req := &LikeRequest{
			TargetType: TargetTypePost,
			TargetID:   postID,
			UserID:     userID,
		}
		_, err := repo.AddLike(ctx, req)
		require.NoError(t, err)

		// Check boolean status
		liked, err := repo.HasUserLiked(ctx, TargetTypePost, postID, userID)
		require.NoError(t, err)
		assert.True(t, liked)

		// Check counter
		count, err := repo.GetLikeCount(ctx, TargetTypePost, postID)
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("Remove Like", func(t *testing.T) {
		req := &LikeRequest{
			TargetType: TargetTypePost,
			TargetID:   postID,
			UserID:     userID,
		}
		err := repo.RemoveLike(ctx, req)
		require.NoError(t, err)

		// Check boolean status
		liked, err := repo.HasUserLiked(ctx, TargetTypePost, postID, userID)
		require.NoError(t, err)
		assert.False(t, liked)

		// Check counter
		count, err := repo.GetLikeCount(ctx, TargetTypePost, postID)
		require.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})
}
