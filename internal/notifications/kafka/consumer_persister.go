package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	kafkago "github.com/segmentio/kafka-go"
	"social-geo-go/internal/data"
)

// PersisterHandler handles storing notifications to DB
type PersisterHandler struct {
	notifRepo     *data.NotificationRepository
	redis         *redis.Client
	modRepo       *data.ModerationRepository // to check blocks/mutes
	deviceRepo    *data.DeviceRepository
	kafkaProducer NotificationEventProducer
}

func NewPersisterHandler(notifRepo *data.NotificationRepository, redis *redis.Client, modRepo *data.ModerationRepository, deviceRepo *data.DeviceRepository, producer NotificationEventProducer) *PersisterHandler {
	return &PersisterHandler{
		notifRepo:     notifRepo,
		redis:         redis,
		modRepo:       modRepo,
		deviceRepo:    deviceRepo,
		kafkaProducer: producer,
	}
}

func (h *PersisterHandler) Handle(ctx context.Context, msg kafkago.Message) error {
	var event NotificationEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		return fmt.Errorf("unmarshal error: %w", err)
	}

	// Idempotency guard for at-least-once Kafka delivery.
	// If Redis is unavailable, we continue and rely on downstream behavior.
	if h.redis != nil && event.EventID != "" {
		dedupeKey := fmt.Sprintf("notif:dedupe:persister:%s", event.EventID)
		ok, err := h.redis.SetNX(ctx, dedupeKey, "1", 24*time.Hour).Result()
		if err != nil {
			slog.Warn("persister dedupe check failed", "event_id", event.EventID, "error", err)
		} else if !ok {
			return nil
		}
	}

	// 1. Check blocks/mutes
	if h.modRepo != nil {
		blocked, _ := h.modRepo.IsBlocked(ctx, event.ActorID, event.RecipientID)
		if blocked {
			// Suppress notification if recipient blocked actor
			return nil
		}
	}

	payload := make(map[string]string, len(event.Payload)+1)
	for k, v := range event.Payload {
		payload[k] = v
	}
	if event.EventID != "" {
		payload["event_id"] = event.EventID
	}

	// 2. Write to Cassandra
	req := &data.CreateNotificationRequest{
		UserID:     event.RecipientID,
		Type:       event.EventType,
		ActorID:    event.ActorID,
		TargetType: event.TargetType,
		TargetID:   event.TargetID,
		Message:    event.Message,
		Payload:    payload,
	}
	notification, err := h.notifRepo.CreateNotification(ctx, req)
	if err != nil {
		return err // don't commit -> retry
	}

	// 3. Publish fully-shaped notification for SSE clients.
	if h.redis != nil {
		channel := fmt.Sprintf("sse:user:%s", event.RecipientID)
		raw, err := json.Marshal(notification)
		if err != nil {
			slog.Warn("failed to marshal sse notification payload", "event_id", event.EventID, "error", err)
		} else {
			if err := h.redis.Publish(ctx, channel, raw).Err(); err != nil {
				slog.Warn("failed to publish sse notification payload", "event_id", event.EventID, "error", err)
			}
		}
	}

	// 4. Fetch device tokens and queue push notifications
	if h.deviceRepo != nil && h.kafkaProducer != nil {
		tokens, _ := h.deviceRepo.GetDeviceTokens(ctx, event.RecipientID)
		if len(tokens) > 0 {
			// Determine badge count
			badgeCount, _ := h.notifRepo.GetUnreadCount(ctx, event.RecipientID)

			pushData := make(map[string]string, len(payload)+6)
			for k, v := range payload {
				pushData[k] = v
			}
			pushData["notification_id"] = notification.ID
			pushData["notification_type"] = notification.Type
			pushData["actor_id"] = notification.ActorID
			pushData["target_type"] = notification.TargetType
			pushData["target_id"] = notification.TargetID
			pushData["badge_count"] = fmt.Sprintf("%d", badgeCount)

			job := &PushDispatchJob{
				EventID:      event.EventID,
				UserID:       event.RecipientID,
				DeviceTokens: tokens,
				Title:        "Geoloc", // You could customize this based on event.EventType
				Body:         event.Message,
				Data:         pushData,
				BadgeCount:   badgeCount,
				RetryCount:   0,
			}
			
			// Fire and forget push dispatch. If it fails to write to Kafka here, 
			// the worst case is a missed push notification (but the in-app notification is already saved).
			_ = h.kafkaProducer.ProducePushDispatch(ctx, job)
		}
	}

	return nil
}
