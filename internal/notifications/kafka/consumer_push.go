package kafka

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	kafkago "github.com/segmentio/kafka-go"
	"social-geo-go/internal/push"
)

type PushDispatchHandler struct {
	pushService   push.PushService
	kafkaProducer NotificationEventProducer
	redis         *redis.Client
}

func NewPushDispatchHandler(pushService push.PushService, producer NotificationEventProducer, redisClient *redis.Client) *PushDispatchHandler {
	return &PushDispatchHandler{
		pushService:   pushService,
		kafkaProducer: producer,
		redis:         redisClient,
	}
}

func (h *PushDispatchHandler) Handle(ctx context.Context, msg kafkago.Message) error {
	var job PushDispatchJob
	if err := json.Unmarshal(msg.Value, &job); err != nil {
		return fmt.Errorf("unmarshal error: %w", err)
	}

	for _, token := range job.DeviceTokens {
		dedupeKey, dedupeEnabled, dedupeLocked, dedupeErr := h.acquirePushDedupeLock(ctx, job.EventID, token)
		if dedupeErr != nil {
			slog.Warn("push dedupe lock failed; continuing send", "event_id", job.EventID, "error", dedupeErr)
		} else if dedupeEnabled && !dedupeLocked {
			slog.Debug("push duplicate skipped", "event_id", job.EventID)
			continue
		}

		err := h.pushService.Send(ctx, token, job.Title, job.Body, job.Data)
		if err != nil {
			slog.Error("Failed to send push", "token", token, "error", err)
			if dedupeLocked {
				h.releasePushDedupeLock(ctx, dedupeKey)
			}
			
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
			continue
		}

		if dedupeLocked {
			h.markPushDedupeDelivered(ctx, dedupeKey)
		}
	}

	return nil
}

func tokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:8])
}

func (h *PushDispatchHandler) pushDedupeKey(eventID, token string) string {
	return fmt.Sprintf("notif:dedupe:push:%s:%s", eventID, tokenDigest(token))
}

// acquirePushDedupeLock obtains a short-lived lock per event/token.
func (h *PushDispatchHandler) acquirePushDedupeLock(ctx context.Context, eventID, token string) (string, bool, bool, error) {
	if h.redis == nil || eventID == "" {
		return "", false, false, nil
	}
	key := h.pushDedupeKey(eventID, token)
	ok, err := h.redis.SetNX(ctx, key, "inflight", 10*time.Minute).Result()
	return key, true, ok, err
}

func (h *PushDispatchHandler) releasePushDedupeLock(ctx context.Context, key string) {
	if h.redis == nil || key == "" {
		return
	}
	_ = h.redis.Del(ctx, key).Err()
}

func (h *PushDispatchHandler) markPushDedupeDelivered(ctx context.Context, key string) {
	if h.redis == nil || key == "" {
		return
	}
	_ = h.redis.Set(ctx, key, "sent", 24*time.Hour).Err()
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
