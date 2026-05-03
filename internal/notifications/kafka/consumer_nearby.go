package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/gocql/gocql"
	"github.com/mmcloughlin/geohash"
	kafkago "github.com/segmentio/kafka-go"
	"social-geo-go/internal/data"
)

type NearbyFanoutHandler struct {
	locFollowRepo *data.LocationFollowRepository
	kafkaProducer NotificationEventProducer
}

func NewNearbyFanoutHandler(locFollowRepo *data.LocationFollowRepository, producer NotificationEventProducer) *NearbyFanoutHandler {
	return &NearbyFanoutHandler{
		locFollowRepo: locFollowRepo,
		kafkaProducer: producer,
	}
}

func (h *NearbyFanoutHandler) Handle(ctx context.Context, msg kafkago.Message) error {
	var job NearbyFanoutJob
	if err := json.Unmarshal(msg.Value, &job); err != nil {
		return fmt.Errorf("unmarshal error: %w", err)
	}

	geohashPrefix := job.Geohash
	if len(geohashPrefix) > 5 {
		geohashPrefix = geohashPrefix[:5]
	}
	
	neighbors := geohash.Neighbors(geohashPrefix)
	allGeohashes := append([]string{geohashPrefix}, neighbors...)

	notifiedUsers := make(map[string]bool)
	notifiedUsers[job.AuthorID] = true // Don't notify the author

	for _, prefix := range allGeohashes {
		users, err := h.locFollowRepo.GetUsersFollowingLocation(ctx, prefix)
		if err != nil {
			slog.Warn("failed to fetch users for geohash", "geohash", prefix, "error", err)
			continue
		}

		for _, userID := range users {
			if notifiedUsers[userID] {
				continue
			}
			notifiedUsers[userID] = true

			event := &NotificationEvent{
				EventID:     gocql.TimeUUID().String(),
				EventType:   data.NotificationTypeLocationPost,
				ActorID:     job.AuthorID,
				RecipientID: userID,
				TargetType:  data.TargetTypePost,
				TargetID:    job.PostID,
				Message:     "New post nearby",
				Payload:     map[string]string{"post_preview": job.Content, "geohash": job.Geohash},
				CreatedAt:   time.Now().Format(time.RFC3339),
			}

			if err := h.kafkaProducer.ProduceNotificationEvent(ctx, event); err != nil {
				slog.Error("failed to produce nearby notification", "user", userID, "error", err)
			}
		}
	}

	return nil
}
