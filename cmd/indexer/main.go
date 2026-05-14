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
	}

	ctx := context.Background()
	if err := search.EnsureESIndexes(ctx, esURL, postsIndex, usersIndex); err != nil {
		slog.Error("failed to ensure ES indexes, continuing anyway", "error", err)
	}

	groupID := "search-indexer"
	topic := "posts.created"

	slog.Info("connecting to Kafka", "brokers", brokers, "group", groupID, "topic", topic)

	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        brokers,
		GroupID:        groupID,
		Topic:          topic,
		MinBytes:       1,
		MaxBytes:       10e6,
		MaxWait:        1 * time.Second,
		CommitInterval: 0,
		ErrorLogger: kafkago.LoggerFunc(func(msg string, args ...interface{}) {
			slog.Error("kafka reader error", "msg", msg, "args", args)
		}),
	})
	defer reader.Close()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	slog.Info("indexer consumer started, waiting for messages")

	for {
		select {
		case <-sigCh:
			slog.Info("shutting down indexer")
			return
		default:
		}

		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("kafka fetch error", "error", err)
			backoffSleep(ctx)
			continue
		}

		if err := processMessage(ctx, esClient, rdb, msg, postsIndex); err != nil {
			slog.Error("failed to process message, will retry on next fetch",
				"partition", msg.Partition,
				"offset", msg.Offset,
				"error", err,
			)
			backoffSleep(ctx)
			continue
		}

		if err := reader.CommitMessages(ctx, msg); err != nil {
			slog.Error("failed to commit message", "error", err)
		}
	}
}

func processMessage(ctx context.Context, esClient *search.ESClient, rdb *redis.Client, msg kafkago.Message, postsIndex string) error {
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

func backoffSleep(ctx context.Context) {
	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
