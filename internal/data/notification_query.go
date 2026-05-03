package data

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"
	"github.com/redis/go-redis/v9"
)

type NotificationRepository struct {
	session *gocql.Session
	redis   *redis.Client
}

func NewNotificationRepository(session *gocql.Session, redisClient *redis.Client) *NotificationRepository {
	return &NotificationRepository{session: session, redis: redisClient}
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
		INSERT INTO notifications_by_user (user_id, notification_id, type, actor_id, target_type, target_id, message, payload, is_read, is_deleted, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, notificationID, req.Type, actorID, req.TargetType, targetID, req.Message, req.Payload, false, false, now).
		WithContext(ctx).Exec()
	if err != nil {
		return nil, fmt.Errorf("failed to create notification: %w", err)
	}

	// Increment unread count in Redis
	if r.redis != nil {
		key := fmt.Sprintf("notif:unread:%s", req.UserID)
		r.redis.Incr(ctx, key)
	}

	return &Notification{
		ID:         notificationID.String(),
		UserID:     req.UserID,
		Type:       req.Type,
		ActorID:    req.ActorID,
		TargetType: req.TargetType,
		TargetID:   req.TargetID,
		Message:    req.Message,
		Payload:    req.Payload,
		IsRead:     false,
		IsDeleted:  false,
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

	query := `SELECT notification_id, type, actor_id, target_type, target_id, message, payload, is_read, is_deleted, created_at
		FROM notifications_by_user WHERE user_id = ? LIMIT ?`

	iter := r.session.Query(query, uid, limit).WithContext(ctx).Iter()

	var notifications []Notification
	var notificationID, actorID, targetID gocql.UUID
	var notifType, targetType, message string
	var payload map[string]string
	var isRead, isDeleted bool
	var createdAt time.Time

	for iter.Scan(&notificationID, &notifType, &actorID, &targetType, &targetID, &message, &payload, &isRead, &isDeleted, &createdAt) {
		if isDeleted {
			continue
		}
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
			Payload:    payload,
			IsRead:     isRead,
			IsDeleted:  isDeleted,
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
	var isRead bool
	err = r.session.Query(`
		SELECT created_at, is_read FROM notifications_by_user 
		WHERE user_id = ? AND notification_id = ? ALLOW FILTERING
	`, uid, nid).WithContext(ctx).Scan(&createdAt, &isRead)
	if err != nil {
		return fmt.Errorf("notification not found")
	}

	if isRead {
		return nil // Already read
	}

	err = r.session.Query(`
		UPDATE notifications_by_user SET is_read = true
		WHERE user_id = ? AND created_at = ? AND notification_id = ?
	`, uid, createdAt, nid).WithContext(ctx).Exec()
	if err != nil {
		return err
	}

	// Decrement unread count in Redis
	if r.redis != nil {
		key := fmt.Sprintf("notif:unread:%s", userID)
		// Use Lua script to prevent negative counts
		script := redis.NewScript(`
			local count = redis.call('GET', KEYS[1])
			if count and tonumber(count) > 0 then
				return redis.call('DECR', KEYS[1])
			else
				return 0
			end
		`)
		script.Run(ctx, r.redis, []string{key})
	}

	return nil
}

// MarkAllAsRead marks all notifications as read for a user
func (r *NotificationRepository) MarkAllAsRead(ctx context.Context, userID string) error {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	// Get all unread notification IDs and created_at
	iter := r.session.Query(`
		SELECT notification_id, created_at FROM notifications_by_user 
		WHERE user_id = ? AND is_read = false ALLOW FILTERING
	`, uid).WithContext(ctx).Iter()

	var nid gocql.UUID
	var createdAt time.Time

	for iter.Scan(&nid, &createdAt) {
		r.session.Query(`
			UPDATE notifications_by_user SET is_read = true
			WHERE user_id = ? AND created_at = ? AND notification_id = ?
		`, uid, createdAt, nid).WithContext(ctx).Exec() //nolint:errcheck
	}

	err = iter.Close()
	if err != nil {
		return err
	}

	// Set unread count to 0 in Redis
	if r.redis != nil {
		key := fmt.Sprintf("notif:unread:%s", userID)
		r.redis.Set(ctx, key, 0, 0)
	}

	return nil
}

// GetUnreadCount returns the count of unread notifications
func (r *NotificationRepository) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	// Fast path: Redis
	if r.redis != nil {
		key := fmt.Sprintf("notif:unread:%s", userID)
		count, err := r.redis.Get(ctx, key).Int()
		if err == nil {
			return count, nil
		}
		// If key doesn't exist or error, fallback to Cassandra
	}

	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return 0, fmt.Errorf("invalid user_id: %w", err)
	}

	var count int
	err = r.session.Query(`
		SELECT COUNT(*) FROM notifications_by_user WHERE user_id = ? AND is_read = false ALLOW FILTERING
	`, uid).WithContext(ctx).Scan(&count)
	if err != nil {
		return 0, err
	}

	// Backfill Redis
	if r.redis != nil {
		key := fmt.Sprintf("notif:unread:%s", userID)
		r.redis.Set(ctx, key, count, 0)
	}

	return count, nil
}

// DeleteNotification soft-deletes a notification
func (r *NotificationRepository) DeleteNotification(ctx context.Context, userID, notificationID string) error {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	nid, err := gocql.ParseUUID(notificationID)
	if err != nil {
		return fmt.Errorf("invalid notification_id: %w", err)
	}

	// Get created_at and is_read for the update
	var createdAt time.Time
	var isRead bool
	err = r.session.Query(`
		SELECT created_at, is_read FROM notifications_by_user 
		WHERE user_id = ? AND notification_id = ? ALLOW FILTERING
	`, uid, nid).WithContext(ctx).Scan(&createdAt, &isRead)
	if err != nil {
		return fmt.Errorf("notification not found")
	}

	err = r.session.Query(`
		UPDATE notifications_by_user SET is_deleted = true
		WHERE user_id = ? AND created_at = ? AND notification_id = ?
	`, uid, createdAt, nid).WithContext(ctx).Exec()
	if err != nil {
		return err
	}

	// If the notification was unread, decrement the unread count
	if !isRead && r.redis != nil {
		key := fmt.Sprintf("notif:unread:%s", userID)
		script := redis.NewScript(`
			local count = redis.call('GET', KEYS[1])
			if count and tonumber(count) > 0 then
				return redis.call('DECR', KEYS[1])
			else
				return 0
			end
		`)
		script.Run(ctx, r.redis, []string{key})
	}

	return nil
}

