package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
	"github.com/redis/go-redis/v9"
	kafkago "github.com/segmentio/kafka-go"

	"social-geo-go/internal/search"
)

func main() {
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "development"
	}
	if err := godotenv.Load(".env." + appEnv); err != nil {
		log.Printf("No .env.%s file found", appEnv)
	}
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	slog.SetDefault(slog.New(
		tint.NewHandler(os.Stdout, &tint.Options{
			Level: slog.LevelDebug,
		}),
	))

	slog.Info("starting search indexer", "APP_ENV", appEnv)

	brokersStr := os.Getenv("KAFKA_BROKERS")
	if brokersStr == "" {
		brokersStr = "localhost:9092"
	}
	brokers := strings.Split(brokersStr, ",")

	esURL := os.Getenv("ELASTICSEARCH_URL")
	if esURL == "" {
		esURL = "http://localhost:9200"
	}

	redisAddr := fmt.Sprintf("%s:%s",
		os.Getenv("REDIS_HOST"),
		os.Getenv("REDIS_PORT"),
	)
	if os.Getenv("REDIS_HOST") == "" {
		redisAddr = "localhost:6379"
	}

	postsIndex := os.Getenv("ELASTICSEARCH_INDEX_POSTS")
	if postsIndex == "" {
		postsIndex = "posts"
	}
	usersIndex := os.Getenv("ELASTICSEARCH_INDEX_USERS")
	if usersIndex == "" {
		usersIndex = "users"
	}

	esClient := search.NewESClient()

	rdb := redis.NewClient(&redis.Options{
		Addr:         redisAddr,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 2,
	})
	defer rdb.Close()

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Warn("redis unavailable, autocomplete sync will be disabled", "error", err)
		rdb = nil
	}

	ctx := context.Background()
	if err := search.EnsureESIndexes(ctx, esURL, postsIndex, usersIndex); err != nil {
		slog.Error("failed to ensure ES indexes, continuing anyway", "error", err)
	}

	groupID := "search-indexer"

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	consumerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		<-sigCh
		slog.Info("shutting down indexer")
		cancel()
	}()

	slog.Info("indexer consumers starting", "group", groupID, "topics", []string{"posts.created", "users.indexed"})

	errCh := make(chan error, 2)
	go func() {
		errCh <- runConsumer(consumerCtx, brokers, groupID, "posts.created", func(msg kafkago.Message) error {
			return processPostMessage(consumerCtx, esClient, rdb, msg, postsIndex)
		})
	}()
	go func() {
		errCh <- runConsumer(consumerCtx, brokers, groupID, "users.indexed", func(msg kafkago.Message) error {
			return processUserMessage(consumerCtx, esClient, rdb, msg, usersIndex)
		})
	}()

	<-errCh
}

func runConsumer(ctx context.Context, brokers []string, groupID, topic string, handler func(kafkago.Message) error) error {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        brokers,
		GroupID:        groupID,
		Topic:          topic,
		MinBytes:       1,
		MaxBytes:       10e6,
		MaxWait:        1 * time.Second,
		CommitInterval: 0,
		ErrorLogger: kafkago.LoggerFunc(func(msg string, args ...interface{}) {
			slog.Error("kafka reader error", "topic", topic, "msg", msg, "args", args)
		}),
	})
	defer reader.Close()

	slog.Info("consumer started", "topic", topic)

	for {
		if ctx.Err() != nil {
			return nil
		}

		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.Error("kafka fetch error", "topic", topic, "error", err)
			backoffSleep(ctx)
			continue
		}

		if err := handler(msg); err != nil {
			slog.Error("failed to process message, will retry on next fetch",
				"topic", topic,
				"partition", msg.Partition,
				"offset", msg.Offset,
				"error", err,
			)
			backoffSleep(ctx)
			continue
		}

		if err := reader.CommitMessages(ctx, msg); err != nil {
			slog.Error("failed to commit message", "topic", topic, "error", err)
		}
	}
}

func processPostMessage(ctx context.Context, esClient *search.ESClient, rdb *redis.Client, msg kafkago.Message, postsIndex string) error {
	var event search.PostCreatedEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		return fmt.Errorf("unmarshal event: %w", err)
	}

	slog.Info("indexing post",
		"post_id", event.PostID,
		"user_id", event.UserID,
		"username", event.Username,
		"content_length", len(event.Content),
	)

	doc := search.PostDocumentFromEvent(event)

	if err := esClient.IndexDocument(ctx, postsIndex, event.PostID, doc); err != nil {
		return fmt.Errorf("index document: %w", err)
	}

	if rdb != nil && event.Username != "" {
		member := event.Username + "\xff"
		if err := rdb.ZAdd(ctx, "users:autocomplete", redis.Z{
			Score:  0,
			Member: member,
		}).Err(); err != nil {
			slog.Warn("failed to sync username to redis autocomplete",
				"username", event.Username,
				"error", err,
			)
		}
	}

	slog.Info("successfully indexed post",
		"post_id", event.PostID,
		"username_synced", event.Username != "",
	)
	return nil
}

func processUserMessage(ctx context.Context, esClient *search.ESClient, rdb *redis.Client, msg kafkago.Message, usersIndex string) error {
	var event search.UserIndexedEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		return fmt.Errorf("unmarshal event: %w", err)
	}

	slog.Info("indexing user",
		"user_id", event.UserID,
		"username", event.Username,
	)

	if err := search.IndexUserFromEvent(ctx, esClient, rdb, usersIndex, event); err != nil {
		return fmt.Errorf("index user: %w", err)
	}

	slog.Info("successfully indexed user", "user_id", event.UserID)
	return nil
}

func backoffSleep(ctx context.Context) {
	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
