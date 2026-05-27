package kafka

import (
	"context"
	"time"

	kafkago "github.com/segmentio/kafka-go"
	"log/slog"
)

// DMMessageProducer writes DM fan-out / push pipeline events to Kafka.
type DMMessageProducer struct {
	writer *kafkago.Writer
}

// NewDMMessageProducer creates a producer for the dm_messages topic.
func NewDMMessageProducer(brokers []string) *DMMessageProducer {
	w := &kafkago.Writer{
		Addr:                   kafkago.TCP(brokers...),
		Topic:                  "dm_messages",
		Balancer:               &kafkago.Hash{},
		RequiredAcks:           kafkago.RequireAll,
		Async:                  true,
		BatchTimeout:           10 * time.Millisecond,
		AllowAutoTopicCreation: true,
		ErrorLogger: kafkago.LoggerFunc(func(msg string, args ...interface{}) {
			slog.Error("dm kafka producer error", "msg", msg, "args", args)
		}),
	}
	return &DMMessageProducer{writer: w}
}

// Publish sends a JSON payload to dm_messages partitioned by key (typically recipient user id).
func (p *DMMessageProducer) Publish(ctx context.Context, partitionKey string, value []byte) error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(partitionKey),
		Value: value,
	})
}

// Close releases the writer.
func (p *DMMessageProducer) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
