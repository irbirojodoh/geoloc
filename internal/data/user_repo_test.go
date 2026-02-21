package data

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	/*
		Skipping SearchUsers test pending full SAI test container support
		t.Run("Search Users (SAI Indexed Exact Match)", func(t *testing.T) {
			// Seed users specifically for search
			repo.CreateUser(ctx, &CreateUserRequest{Username: "alpha_one", FullName: "Alpha One", Email: "1@t.com"})     //nolint:errcheck
			repo.CreateUser(ctx, &CreateUserRequest{Username: "beta_two", FullName: "Beta Two", Email: "2@t.com"})       //nolint:errcheck
			repo.CreateUser(ctx, &CreateUserRequest{Username: "alpha_three", FullName: "Alpha Three", Email: "3@t.com"}) //nolint:errcheck

			// Wait briefly for the Storage-Attached Index (SAI) to project the new rows
			// before asserting the search succeeds.
			time.Sleep(1 * time.Second)

			// Test Search for exact match using SAI
			results, err := repo.SearchUsers(ctx, "alpha_one", 10)
			require.NoError(t, err)

			// Should find exactly one record now that we use SAI instead of ALLOW FILTERING client string contains
			assert.Equal(t, 1, len(results))
			assert.Equal(t, "alpha_one", results[0].Username)
		})
	*/
}

func TestUserRepository_OAuth(t *testing.T) {
	repo := NewUserRepository(testSession)
	ctx := context.Background()

	// Shared data for the tests
	email := "oauth_test@example.com"
	fullName := "OAuth Tester"
	avatarURL := "https://example.com/oauth_avatar.jpg"
	var firstUserID string

	t.Run("1. Create New User (First Login)", func(t *testing.T) {
		// Action
		user, isNew, err := repo.GetOrCreateOAuthUser(ctx, email, fullName, avatarURL)

		// Assertions
		require.NoError(t, err)
		assert.True(t, isNew, "Should be marked as a new user")
		assert.NotEmpty(t, user.ID)
		assert.Equal(t, email, user.Email)
		assert.Equal(t, fullName, user.FullName)
		assert.Equal(t, avatarURL, user.ProfilePictureURL)

		// Verify username generation logic (oauth_test_...)
		assert.Contains(t, user.Username, "oauth_test")

		// Verify persistence by querying directly
		savedUser, err := repo.GetUserByID(ctx, user.ID)
		require.NoError(t, err)
		assert.Equal(t, "Joined via Social Login", savedUser.Bio)

		// Store ID for next test
		firstUserID = user.ID
	})

	t.Run("2. Return Existing User (Subsequent Login)", func(t *testing.T) {
		// Action: Login with same email
		// We pass different name/avatar to ensure it doesn't accidentally create a new record
		// Note: Current logic doesn't update profile on login, just retrieves
		user, isNew, err := repo.GetOrCreateOAuthUser(ctx, email, "Different Name", "http://different.url")

		// Assertions
		require.NoError(t, err)
		assert.False(t, isNew, "Should NOT be marked as new user")
		assert.Equal(t, firstUserID, user.ID, "ID should match the existing user")
		assert.Equal(t, email, user.Email)
	})

	t.Run("3. Handle Empty Email (Apple Privacy Edge Case)", func(t *testing.T) {
		// Action
		user, isNew, err := repo.GetOrCreateOAuthUser(ctx, "", "No Email User", "")

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "email is required")
		assert.Nil(t, user)
		assert.False(t, isNew)
	})

	t.Run("4. Handle Username Special Characters", func(t *testing.T) {
		// Email with dots and special chars
		complexEmail := "jane.doe+test@gmail.com"

		user, _, err := repo.GetOrCreateOAuthUser(ctx, complexEmail, "Jane", "")
		require.NoError(t, err)

		// Ensure the username was sanitized (no @ or + allowed usually in simple generation)
		// Our logic maps non-alphanumeric to underscore
		// "jane.doe+test" -> "jane_doe_test..."
		assert.NotContains(t, user.Username, "+")
		assert.NotContains(t, user.Username, "@")
		assert.Contains(t, user.Username, "jane_doe_test")
	})
}
