package notifications

import (
	"context"

	"github.com/redis/go-redis/v9"
	"social-geo-go/internal/data"
	"social-geo-go/internal/notifications/kafka"
)

// NotificationDispatcher routes events to Kafka or direct Cassandra.
type NotificationDispatcher struct {
	kafkaProducer kafka.NotificationEventProducer
	notifRepo     *data.NotificationRepository
	redisClient   *redis.Client
	kafkaEnabled  bool
}

func NewDispatcher(producer kafka.NotificationEventProducer, repo *data.NotificationRepository, redisClient *redis.Client) *NotificationDispatcher {
	return &NotificationDispatcher{
		kafkaProducer: producer,
		notifRepo:     repo,
		redisClient:   redisClient,
		kafkaEnabled:  producer != nil,
	}
}

func (d *NotificationDispatcher) Dispatch(ctx context.Context, event *kafka.NotificationEvent) error {
	if d.kafkaEnabled {
		return d.kafkaProducer.ProduceNotificationEvent(ctx, event)
	}
	
	// Fallback: direct write (Phase 1 / local dev without Kafka)
	req := &data.CreateNotificationRequest{
		UserID:     event.RecipientID,
		Type:       event.EventType,
		ActorID:    event.ActorID,
		TargetType: event.TargetType,
		TargetID:   event.TargetID,
		Message:    event.Message,
		Payload:    event.Payload,
	}
	_, err := d.notifRepo.CreateNotification(ctx, req)
	if err != nil {
		return err
	}

	return nil
}

func (d *NotificationDispatcher) DispatchNearbyFanout(ctx context.Context, job *kafka.NearbyFanoutJob) error {
	if d.kafkaEnabled {
		return d.kafkaProducer.ProduceNearbyFanout(ctx, job)
	}
	
	// Fallback for nearby fanout is omitted in Phase 1 as it would be too slow inline
	return nil
}
