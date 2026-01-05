package data

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUserRepository_Integration(t *testing.T) {
	// Initialize the Repository
	repo := NewUserRepository(testSession)
	ctx := context.Background()

	// =========================================================================
	// TEST SCENARIOS
	// =========================================================================

	t.Run("Create and Get User", func(t *testing.T) {
		req := &CreateUserRequest{
			Username:          "John_Doe",
			Email:             "john_doe@example.com",
			FullName:          "John Doe",
			Bio:               "Software Engineer",
			PhoneNumber:       "+62812345678",
			ProfilePictureURL: "https://example.com/pic.jpg",
			PasswordHash:      "secret_hash",
		}

		// Test Create
		createdUser, err := repo.CreateUser(ctx, req)
		require.NoError(t, err)
		assert.NotEmpty(t, createdUser.ID)
		assert.Equal(t, req.Username, createdUser.Username)

		// Test GetByID
		fetchedUser, err := repo.GetUserByID(ctx, createdUser.ID)
		require.NoError(t, err)
		assert.Equal(t, createdUser.ID, fetchedUser.ID)
		assert.Equal(t, "Software Engineer", fetchedUser.Bio)
	})

	t.Run("Get User By Email (ALLOW FILTERING check)", func(t *testing.T) {
		// Insert a fresh user for this test
		req := &CreateUserRequest{
			Username:     "email_tester",
			Email:        "unique@test.com",
			FullName:     "Tester",
			PasswordHash: "123",
		}
		_, err := repo.CreateUser(ctx, req)
		require.NoError(t, err)

		// Test GetByEmail
		user, err := repo.GetUserByEmail(ctx, "unique@test.com")
		require.NoError(t, err)
		assert.Equal(t, "email_tester", user.Username)
	})

	t.Run("Update User", func(t *testing.T) {
		// Create user
		req := &CreateUserRequest{
			Username: "updater",
			Email:    "update@test.com",
			Bio:      "Old Bio",
		}
		user, _ := repo.CreateUser(ctx, req)

		// Perform Update
		updatedUser, err := repo.UpdateUser(ctx, user.ID, "New Name", "New Bio", "http://new.pic")
		require.NoError(t, err)

		// Assert return value
		assert.Equal(t, "New Bio", updatedUser.Bio)
		assert.Equal(t, "New Name", updatedUser.FullName)

		// Verify in DB again
		check, _ := repo.GetUserByID(ctx, user.ID)
		assert.Equal(t, "New Bio", check.Bio)
	})

	t.Run("Search Users (Client-side filtering check)", func(t *testing.T) {
		// Seed users specifically for search
		repo.CreateUser(ctx, &CreateUserRequest{Username: "alpha_one", FullName: "Alpha One", Email: "1@t.com"})
		repo.CreateUser(ctx, &CreateUserRequest{Username: "beta_two", FullName: "Beta Two", Email: "2@t.com"})
		repo.CreateUser(ctx, &CreateUserRequest{Username: "alpha_three", FullName: "Alpha Three", Email: "3@t.com"})

		// Test Search
		results, err := repo.SearchUsers(ctx, "alpha", 10)
		require.NoError(t, err)

		// Should find "alpha_one" and "alpha_three"
		assert.GreaterOrEqual(t, len(results), 2)

		foundAlphaOne := false
		for _, u := range results {
			if u.Username == "alpha_one" {
				foundAlphaOne = true
			}
		}
		assert.True(t, foundAlphaOne, "Search should retrieve matching users")
	})
}
