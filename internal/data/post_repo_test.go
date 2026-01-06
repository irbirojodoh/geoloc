package data

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostRepository_Integration(t *testing.T) {
	repo := NewPostRepository(testSession)
	ctx := context.Background()

	// Helper to create a user for the posts
	userRepo := NewUserRepository(testSession)
	user, _ := userRepo.CreateUser(ctx, &CreateUserRequest{
		Username: "post_tester", Email: "post@test.com",
	})

	t.Run("Create Post", func(t *testing.T) {
		req := &CreatePostRequest{
			UserID:    user.ID,
			Content:   "Hello World",
			Latitude:  -6.2088, // Jakarta
			Longitude: 106.8456,
			MediaURLs: []string{"http://img.com/1.jpg"},
		}

		post, err := repo.CreatePost(ctx, req)
		require.NoError(t, err)
		assert.NotEmpty(t, post.ID)
		assert.Equal(t, req.Content, post.Content)
		assert.NotEmpty(t, post.Geohash, "Geohash should be generated")
	})

	t.Run("Get Nearby Posts (Geospatial Query)", func(t *testing.T) {
		// 1. Central Point (Monas, Jakarta)
		centerLat, centerLng := -6.1754, 106.8272

		// 2. Insert Post A: Very close (500m away)
		repo.CreatePost(ctx, &CreatePostRequest{
			UserID: user.ID, Content: "Near Post",
			Latitude: -6.1710, Longitude: 106.8272,
		})

		// 3. Insert Post B: Far away (Bandung, ~120km away)
		repo.CreatePost(ctx, &CreatePostRequest{
			UserID: user.ID, Content: "Far Post",
			Latitude: -6.9175, Longitude: 107.6191,
		})

		// 4. Query within 5KM
		posts, err := repo.GetNearbyPosts(ctx, centerLat, centerLng, 5.0, 10, time.Time{})
		require.NoError(t, err)

		// Assertions
		assert.GreaterOrEqual(t, len(posts), 1)

		foundNear := false
		foundFar := false
		for _, p := range posts {
			if p.Content == "Near Post" {
				foundNear = true
			}
			if p.Content == "Far Post" {
				foundFar = true
			}
		}

		assert.True(t, foundNear, "Should find the nearby post")
		assert.False(t, foundFar, "Should NOT find the far post")
	})

	t.Run("Get User Posts", func(t *testing.T) {
		posts, err := repo.GetPostsByUser(ctx, user.ID, 10)
		require.NoError(t, err)
		assert.NotEmpty(t, posts)
	})
}
