package data

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"
)

type NotificationRepository struct {
	session *gocql.Session
}

func NewNotificationRepository(session *gocql.Session) *NotificationRepository {
	return &NotificationRepository{session: session}
}

// CreateNotification creates a new notification
func (r *NotificationRepository) CreateNotification(ctx context.Context, req *CreateNotificationRequest) (*Notification, error) {
	userID, err := gocql.ParseUUID(req.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	actorID, err := gocql.ParseUUID(req.ActorID)
	if err != nil {
		return nil, fmt.Errorf("invalid actor_id: %w", err)
	}

	var targetID gocql.UUID
	if req.TargetID != "" {
		targetID, _ = gocql.ParseUUID(req.TargetID)
	}

	notificationID := gocql.TimeUUID()
	now := time.Now()

	err = r.session.Query(`
		INSERT INTO notifications (user_id, notification_id, type, actor_id, target_type, target_id, message, is_read, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, notificationID, req.Type, actorID, req.TargetType, targetID, req.Message, false, now).
		WithContext(ctx).Exec()
	if err != nil {
		return nil, fmt.Errorf("failed to create notification: %w", err)
	}

	return &Notification{
		ID:         notificationID.String(),
		UserID:     req.UserID,
		Type:       req.Type,
		ActorID:    req.ActorID,
		TargetType: req.TargetType,
		TargetID:   req.TargetID,
		Message:    req.Message,
		IsRead:     false,
		CreatedAt:  now,
	}, nil
}

// GetNotifications retrieves notifications for a user
func (r *NotificationRepository) GetNotifications(ctx context.Context, userID string, limit int, unreadOnly bool) ([]Notification, error) {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := `SELECT notification_id, type, actor_id, target_type, target_id, message, is_read, created_at
		FROM notifications WHERE user_id = ? LIMIT ?`

	iter := r.session.Query(query, uid, limit).WithContext(ctx).Iter()

	var notifications []Notification
	var notificationID, actorID, targetID gocql.UUID
	var notifType, targetType, message string
	var isRead bool
	var createdAt time.Time

	for iter.Scan(&notificationID, &notifType, &actorID, &targetType, &targetID, &message, &isRead, &createdAt) {
		if unreadOnly && isRead {
			continue
		}
		n := Notification{
			ID:         notificationID.String(),
			UserID:     userID,
			Type:       notifType,
			ActorID:    actorID.String(),
			TargetType: targetType,
			Message:    message,
			IsRead:     isRead,
			CreatedAt:  createdAt,
		}
		if targetID.String() != "00000000-0000-0000-0000-000000000000" {
			n.TargetID = targetID.String()
		}
		notifications = append(notifications, n)
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	return notifications, nil
}

// MarkAsRead marks a notification as read
func (r *NotificationRepository) MarkAsRead(ctx context.Context, userID, notificationID string) error {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	nid, err := gocql.ParseUUID(notificationID)
	if err != nil {
		return fmt.Errorf("invalid notification_id: %w", err)
	}

	// Get created_at for the update
	var createdAt time.Time
	err = r.session.Query(`
		SELECT created_at FROM notifications 
		WHERE user_id = ? AND notification_id = ? ALLOW FILTERING
	`, uid, nid).WithContext(ctx).Scan(&createdAt)
	if err != nil {
		return fmt.Errorf("notification not found")
	}

	return r.session.Query(`
		UPDATE notifications SET is_read = true
		WHERE user_id = ? AND created_at = ? AND notification_id = ?
	`, uid, createdAt, nid).WithContext(ctx).Exec()
}

// MarkAllAsRead marks all notifications as read for a user
func (r *NotificationRepository) MarkAllAsRead(ctx context.Context, userID string) error {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	// Get all unread notification IDs and created_at
	iter := r.session.Query(`
		SELECT notification_id, created_at FROM notifications 
		WHERE user_id = ? AND is_read = false ALLOW FILTERING
	`, uid).WithContext(ctx).Iter()

	var nid gocql.UUID
	var createdAt time.Time

	for iter.Scan(&nid, &createdAt) {
		r.session.Query(`
			UPDATE notifications SET is_read = true
			WHERE user_id = ? AND created_at = ? AND notification_id = ?
		`, uid, createdAt, nid).WithContext(ctx).Exec() //nolint:errcheck
	}

	return iter.Close()
}

// GetUnreadCount returns the count of unread notifications
func (r *NotificationRepository) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return 0, fmt.Errorf("invalid user_id: %w", err)
	}

	var count int
	err = r.session.Query(`
		SELECT COUNT(*) FROM notifications WHERE user_id = ? AND is_read = false ALLOW FILTERING
	`, uid).WithContext(ctx).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}
