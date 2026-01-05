package data

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFollowRepository_Integration(t *testing.T) {
	repo := NewFollowRepository(testSession)
	ctx := context.Background()

	userA := uuid.New().String() // The Follower
	userB := uuid.New().String() // The Target

	t.Run("Follow User", func(t *testing.T) {
		err := repo.Follow(ctx, userA, userB)
		require.NoError(t, err)

		isFollowing, err := repo.IsFollowing(ctx, userA, userB)
		require.NoError(t, err)
		assert.True(t, isFollowing)
	})

	t.Run("Verify Lists", func(t *testing.T) {
		// Does A follow B?
		following, err := repo.GetFollowing(ctx, userA, 10)
		require.NoError(t, err)
		assert.Contains(t, following, userB)

		// Is A in B's followers?
		followers, err := repo.GetFollowers(ctx, userB, 10)
		require.NoError(t, err)
		assert.Contains(t, followers, userA)
	})

	t.Run("Verify Counts", func(t *testing.T) {
		countsA, err := repo.GetFollowCounts(ctx, userA)
		require.NoError(t, err)
		assert.Equal(t, int64(1), countsA.FollowingCount)

		countsB, err := repo.GetFollowCounts(ctx, userB)
		require.NoError(t, err)
		assert.Equal(t, int64(1), countsB.FollowersCount)
	})

	t.Run("Unfollow", func(t *testing.T) {
		err := repo.Unfollow(ctx, userA, userB)
		require.NoError(t, err)

		isFollowing, _ := repo.IsFollowing(ctx, userA, userB)
		assert.False(t, isFollowing)
	})
}
