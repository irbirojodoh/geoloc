package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
	kafkago "github.com/segmentio/kafka-go"
)

// SSEFanoutHandler publishes notifications to Redis Pub/Sub for SSE delivery
type SSEFanoutHandler struct {
	redis *redis.Client
}

func NewSSEFanoutHandler(redis *redis.Client) *SSEFanoutHandler {
	return &SSEFanoutHandler{redis: redis}
}

func (h *SSEFanoutHandler) Handle(ctx context.Context, msg kafkago.Message) error {
	var event NotificationEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		return fmt.Errorf("unmarshal error: %w", err)
	}

	channel := fmt.Sprintf("sse:user:%s", event.RecipientID)
	
	// Re-marshal to send over pub/sub
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// Publish to Redis Pub/Sub - fire and forget
	return h.redis.Publish(ctx, channel, payload).Err()
}
