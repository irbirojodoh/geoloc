package data

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommentRepository_Integration(t *testing.T) {
	repo := NewCommentRepository(testSession)
	postRepo := NewPostRepository(testSession)
	userRepo := NewUserRepository(testSession)
	ctx := context.Background()

	// Setup data
	user, err := userRepo.CreateUser(ctx, &CreateUserRequest{Username: "commenter", Email: "c@t.com"})
	require.NoError(t, err)
	post, err := postRepo.CreatePost(ctx, &CreatePostRequest{UserID: user.ID, Content: "Topic", Latitude: 0, Longitude: 0})
	require.NoError(t, err)

	var rootComment *Comment

	t.Run("Create Root Comment", func(t *testing.T) {
		req := &CreateCommentRequest{
			PostID: post.ID, UserID: user.ID, Content: "Root Comment",
		}
		c, err := repo.CreateComment(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, 1, c.Depth)
		rootComment = c
	})

	t.Run("Create Nested Reply (Depth 2)", func(t *testing.T) {
		req := &CreateCommentRequest{
			PostID: post.ID, UserID: user.ID, Content: "Reply to Root",
			ParentID: rootComment.ID,
		}
		c, err := repo.CreateComment(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, 2, c.Depth)
		assert.Equal(t, rootComment.ID, c.ParentID)
	})

	t.Run("Get Comments Tree", func(t *testing.T) {
		comments, err := repo.GetCommentsForPost(ctx, post.ID, 10)
		require.NoError(t, err)

		// We expect the function to return "roots", containing "replies"
		assert.NotEmpty(t, comments)
		assert.Equal(t, "Root Comment", comments[0].Content)
		assert.NotEmpty(t, comments[0].Replies, "Root should have replies")
		assert.Equal(t, "Reply to Root", comments[0].Replies[0].Content)
	})

	t.Run("Comment Count Update", func(t *testing.T) {
		count, err := repo.GetCommentCount(ctx, post.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(2), count) // Root + Reply
	})
}
