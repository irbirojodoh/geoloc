package search

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"social-geo-go/internal/data"
)

const publishTimeout = 5 * time.Second

// UserIndexedEventFromUser builds a Kafka/ES user index event from a Cassandra user row.
func UserIndexedEventFromUser(user *data.User, followerCount int) UserIndexedEvent {
	if user == nil {
		return UserIndexedEvent{}
	}
	return UserIndexedEvent{
		UserID:        user.ID,
		Username:      user.Username,
		DisplayName:   user.FullName,
		FollowerCount: followerCount,
		IsVerified:    false,
		AvatarURL:     user.ProfilePictureURL,
	}
}

// IndexUserFromEvent writes a user document to Elasticsearch and syncs autocomplete in Redis.
func IndexUserFromEvent(ctx context.Context, es *ESClient, rdb *redis.Client, usersIndex string, event UserIndexedEvent) error {
	if event.UserID == "" || event.Username == "" {
		return nil
	}

	doc := UserDocumentFromEvent(event)
	if err := es.IndexDocument(ctx, usersIndex, event.UserID, doc); err != nil {
		return err
	}

	if rdb != nil {
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

	return nil
}

// PublishUserIndexedAsync publishes a user index event without blocking the HTTP handler.
func PublishUserIndexedAsync(indexer SearchIndexer, event UserIndexedEvent) {
	if indexer == nil || event.UserID == "" || event.Username == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
		defer cancel()
		if err := indexer.PublishUserIndexed(ctx, &event); err != nil {
			slog.Warn("search: failed to publish user indexed",
				"user_id", event.UserID,
				"error", err,
			)
		}
	}()
}
