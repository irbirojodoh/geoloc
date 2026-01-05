package data

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationRepository_Integration(t *testing.T) {
	repo := NewNotificationRepository(testSession)
	ctx := context.Background()

	userID := uuid.New().String()
	actorID := uuid.New().String()

	t.Run("Create and Get Notifications", func(t *testing.T) {
		req := &CreateNotificationRequest{
			UserID: userID, ActorID: actorID, Type: "like", Message: "Liked your post",
		}
		notif, err := repo.CreateNotification(ctx, req)
		require.NoError(t, err)
		assert.False(t, notif.IsRead)

		// Fetch
		list, err := repo.GetNotifications(ctx, userID, 10, false)
		require.NoError(t, err)
		assert.Len(t, list, 1)
		assert.Equal(t, "Liked your post", list[0].Message)
	})

	t.Run("Mark As Read", func(t *testing.T) {
		// Get the notification ID from previous step
		list, _ := repo.GetNotifications(ctx, userID, 1, false)
		notifID := list[0].ID

		err := repo.MarkAsRead(ctx, userID, notifID)
		require.NoError(t, err)

		// Verify count
		count, err := repo.GetUnreadCount(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}
