package kafka

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type MessageHandler func(ctx context.Context, msg kafkago.Message) error

// RunConsumerGroup starts a Kafka consumer with manual offset commit.
func RunConsumerGroup(ctx context.Context, brokers []string, groupID, topic string, handler MessageHandler) {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        brokers,
		GroupID:        groupID,
		Topic:          topic,
		MinBytes:       1e3,   // 1KB
		MaxBytes:       10e6,  // 10MB
		MaxWait:        500 * time.Millisecond,
		CommitInterval: 0,     // manual commit only
	})
	defer reader.Close()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	slog.Info("consumer group started", "group", groupID, "topic", topic)

	for {
		select {
		case <-ctx.Done():
			return
		case <-sigCh:
			slog.Info("consumer shutting down", "group", groupID)
			return
		default:
		}

		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // Context canceled
			}
			slog.Error("fetch error", "group", groupID, "error", err)
			continue
		}

		if err := handler(ctx, msg); err != nil {
			slog.Error("handler error", "group", groupID,
				"partition", msg.Partition, "offset", msg.Offset, "error", err)
			// TODO: After max retries -> route to DLQ
			// For now, if we don't commit, it will be retried when the consumer restarts or rebalances.
			// Or we could sleep and retry inline.
			time.Sleep(1 * time.Second)
			continue
		}

		// Commit only after successful processing
		if err := reader.CommitMessages(ctx, msg); err != nil {
			slog.Error("commit error", "group", groupID, "error", err)
		}
	}
}
