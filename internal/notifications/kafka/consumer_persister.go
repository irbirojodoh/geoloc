package kafka

import (
	"context"
	"encoding/json"
	"fmt"

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

	// 1. Check blocks/mutes
	if h.modRepo != nil {
		blocked, _ := h.modRepo.IsBlocked(ctx, event.ActorID, event.RecipientID)
		if blocked {
			// Suppress notification if recipient blocked actor
			return nil
		}
	}

	// 2. Write to Cassandra
	req := &data.CreateNotificationRequest{
		UserID:     event.RecipientID,
		Type:       event.EventType,
		ActorID:    event.ActorID,
		TargetType: event.TargetType,
		TargetID:   event.TargetID,
		Message:    event.Message,
		Payload:    event.Payload,
	}
	_, err := h.notifRepo.CreateNotification(ctx, req)
	if err != nil {
		return err // don't commit -> retry
	}

	// 3. Unread count increment is now handled inside CreateNotification for Phase 1 compatibility, 
	// but if we were to strictly decouple it, we would do it here. Since CreateNotification already 
	// does it, we don't need to do it twice.

	// 4. Fetch device tokens and queue push notifications
	if h.deviceRepo != nil && h.kafkaProducer != nil {
		tokens, _ := h.deviceRepo.GetDeviceTokens(ctx, event.RecipientID)
		if len(tokens) > 0 {
			// Determine badge count
			badgeCount, _ := h.notifRepo.GetUnreadCount(ctx, event.RecipientID)

			job := &PushDispatchJob{
				EventID:      event.EventID,
				UserID:       event.RecipientID,
				DeviceTokens: tokens,
				Title:        "Geoloc", // You could customize this based on event.EventType
				Body:         event.Message,
				Data:         event.Payload,
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
