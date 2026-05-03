package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	kafkago "github.com/segmentio/kafka-go"
	"social-geo-go/internal/push"
)

type PushDispatchHandler struct {
	pushService   push.PushService
	kafkaProducer NotificationEventProducer
}

func NewPushDispatchHandler(pushService push.PushService, producer NotificationEventProducer) *PushDispatchHandler {
	return &PushDispatchHandler{
		pushService:   pushService,
		kafkaProducer: producer,
	}
}

func (h *PushDispatchHandler) Handle(ctx context.Context, msg kafkago.Message) error {
	var job PushDispatchJob
	if err := json.Unmarshal(msg.Value, &job); err != nil {
		return fmt.Errorf("unmarshal error: %w", err)
	}

	for _, token := range job.DeviceTokens {
		err := h.pushService.Send(ctx, token, job.Title, job.Body, job.Data)
		if err != nil {
			slog.Error("Failed to send push", "token", token, "error", err)
			
			// If it failed and we haven't exceeded retries, send to retry queue
			if job.RetryCount < 3 && h.kafkaProducer != nil {
				retryJob := &PushRetryJob{
					PushDispatchJob: job,
					LastError:       err.Error(),
					RetryAfter:      time.Now().Add(5 * time.Minute).Format(time.RFC3339),
					MaxRetries:      3,
				}
				retryJob.PushDispatchJob.RetryCount++
				// Just route the failing token
				retryJob.DeviceTokens = []string{token}
				
				_ = h.kafkaProducer.ProducePushRetry(ctx, retryJob)
			}
		}
	}

	return nil
}

// PushRetryHandler processes retries with delay
type PushRetryHandler struct {
	pushService   push.PushService
	kafkaProducer NotificationEventProducer
}

func NewPushRetryHandler(pushService push.PushService, producer NotificationEventProducer) *PushRetryHandler {
	return &PushRetryHandler{
		pushService:   pushService,
		kafkaProducer: producer,
	}
}

func (h *PushRetryHandler) Handle(ctx context.Context, msg kafkago.Message) error {
	var job PushRetryJob
	if err := json.Unmarshal(msg.Value, &job); err != nil {
		return fmt.Errorf("unmarshal error: %w", err)
	}

	// Check if it's time to retry
	retryAfter, _ := time.Parse(time.RFC3339, job.RetryAfter)
	if time.Now().Before(retryAfter) {
		// Not time yet, sleep a bit so we don't spin (in a real system, DLQ or delay exchange is better)
		time.Sleep(1 * time.Second)
		return fmt.Errorf("not time to retry yet") // Return error to prevent commit, will consume again
	}

	// Send it back to the dispatch queue
	return h.kafkaProducer.ProducePushDispatch(ctx, &job.PushDispatchJob)
}
