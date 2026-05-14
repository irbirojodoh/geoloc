package search

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

const postsCreatedTopic = "posts.created"

// PostIndexer publishes post-created events for the search indexer.
type PostIndexer interface {
	PublishPostCreated(ctx context.Context, event *PostCreatedEvent) error
	Close() error
}

type kafkaPostIndexer struct {
	writer *kafkago.Writer
}

// NewPostIndexer creates a Kafka producer for the posts.created topic.
func NewPostIndexer(brokers []string) PostIndexer {
	return &kafkaPostIndexer{
		writer: &kafkago.Writer{
			Addr:                   kafkago.TCP(brokers...),
			Topic:                  postsCreatedTopic,
			Balancer:               &kafkago.Hash{},
			RequiredAcks:           kafkago.RequireAll,
			Async:                  true,
			BatchTimeout:           10 * time.Millisecond,
			AllowAutoTopicCreation: true,
			ErrorLogger: kafkago.LoggerFunc(func(msg string, args ...interface{}) {
				slog.Error("search kafka producer error", "msg", msg, "args", args)
			}),
		},
	}
}

func (p *kafkaPostIndexer) PublishPostCreated(ctx context.Context, event *PostCreatedEvent) error {
	value, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(event.PostID),
		Value: value,
	})
}

func (p *kafkaPostIndexer) Close() error {
	return p.writer.Close()
}
