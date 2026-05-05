package kafka

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

// NotificationEventProducer publishes notification events to Kafka.
type NotificationEventProducer interface {
	ProduceNotificationEvent(ctx context.Context, event *NotificationEvent) error
	ProduceNearbyFanout(ctx context.Context, job *NearbyFanoutJob) error
	ProducePushDispatch(ctx context.Context, job *PushDispatchJob) error
	ProducePushRetry(ctx context.Context, job *PushRetryJob) error
	Close() error
}

type kafkaProducer struct {
	eventsWriter *kafkago.Writer
	nearbyWriter *kafkago.Writer
	pushWriter   *kafkago.Writer
	retryWriter  *kafkago.Writer
}

func NewNotificationEventProducer(brokers []string) NotificationEventProducer {
	makeWriter := func(topic string, async bool) *kafkago.Writer {
		return &kafkago.Writer{
			Addr:         kafkago.TCP(brokers...),
			Topic:        topic,
			Balancer:               &kafkago.Hash{}, // partition by key
			RequiredAcks:           kafkago.RequireAll,
			Async:                  async,
			BatchTimeout:           10 * time.Millisecond,
			AllowAutoTopicCreation: true,
			ErrorLogger: kafkago.LoggerFunc(func(msg string, args ...interface{}) {
				slog.Error("kafka producer error", "msg", msg, "args", args)
			}),
		}
	}

	return &kafkaProducer{
		eventsWriter: makeWriter("notification.events", true),         // async
		nearbyWriter: makeWriter("notification.nearby.fanout", false), // sync
		pushWriter:   makeWriter("notification.push.dispatch", true),
		retryWriter:  makeWriter("notification.push.retry", true),
	}
}

func (p *kafkaProducer) ProduceNotificationEvent(ctx context.Context, event *NotificationEvent) error {
	value, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.eventsWriter.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(event.RecipientID), // partition by recipient
		Value: value,
	})
}

func (p *kafkaProducer) ProduceNearbyFanout(ctx context.Context, job *NearbyFanoutJob) error {
	value, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return p.nearbyWriter.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(job.Geohash),
		Value: value,
	})
}

func (p *kafkaProducer) ProducePushDispatch(ctx context.Context, job *PushDispatchJob) error {
	value, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return p.pushWriter.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(job.UserID),
		Value: value,
	})
}

func (p *kafkaProducer) ProducePushRetry(ctx context.Context, job *PushRetryJob) error {
	value, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return p.retryWriter.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(job.UserID),
		Value: value,
	})
}

func (p *kafkaProducer) Close() error {
	var err error
	if e := p.eventsWriter.Close(); e != nil {
		err = e
	}
	if e := p.nearbyWriter.Close(); e != nil {
		err = e
	}
	if e := p.pushWriter.Close(); e != nil {
		err = e
	}
	if e := p.retryWriter.Close(); e != nil {
		err = e
	}
	return err
}
