package search

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

const (
	postsCreatedTopic = "posts.created"
	usersIndexedTopic = "users.indexed"
)

// SearchIndexer publishes search indexing events for the indexer workers.
type SearchIndexer interface {
	PublishPostCreated(ctx context.Context, event *PostCreatedEvent) error
	PublishUserIndexed(ctx context.Context, event *UserIndexedEvent) error
	Close() error
}

// PostIndexer is kept for call sites that only reference post publishing.
type PostIndexer = SearchIndexer

type kafkaSearchIndexer struct {
	postsWriter *kafkago.Writer
	usersWriter *kafkago.Writer
}

func newKafkaWriter(brokers []string, topic string) *kafkago.Writer {
	return &kafkago.Writer{
		Addr:                   kafkago.TCP(brokers...),
		Topic:                  topic,
		Balancer:               &kafkago.Hash{},
		RequiredAcks:           kafkago.RequireAll,
		Async:                  true,
		BatchTimeout:           10 * time.Millisecond,
		AllowAutoTopicCreation: true,
		ErrorLogger: kafkago.LoggerFunc(func(msg string, args ...interface{}) {
			slog.Error("search kafka producer error", "msg", msg, "args", args)
		}),
	}
}

// NewSearchIndexer creates Kafka producers for posts.created and users.indexed.
func NewSearchIndexer(brokers []string) SearchIndexer {
	return &kafkaSearchIndexer{
		postsWriter: newKafkaWriter(brokers, postsCreatedTopic),
		usersWriter: newKafkaWriter(brokers, usersIndexedTopic),
	}
}

// NewPostIndexer is an alias for NewSearchIndexer.
func NewPostIndexer(brokers []string) PostIndexer {
	return NewSearchIndexer(brokers)
}

func (p *kafkaSearchIndexer) PublishPostCreated(ctx context.Context, event *PostCreatedEvent) error {
	value, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.postsWriter.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(event.PostID),
		Value: value,
	})
}

func (p *kafkaSearchIndexer) PublishUserIndexed(ctx context.Context, event *UserIndexedEvent) error {
	value, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.usersWriter.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(event.UserID),
		Value: value,
	})
}

func (p *kafkaSearchIndexer) Close() error {
	if err := p.postsWriter.Close(); err != nil {
		return err
	}
	return p.usersWriter.Close()
}
