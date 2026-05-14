package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gocql/gocql"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"social-geo-go/internal/data"
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

	host := os.Getenv("CASSANDRA_HOST")
	if host == "" {
		host = "localhost"
	}
	keyspace := os.Getenv("CASSANDRA_KEYSPACE")
	if keyspace == "" {
		keyspace = "geoloc"
	}

	cluster := gocql.NewCluster(host)
	cluster.Keyspace = keyspace
	cluster.Consistency = gocql.Quorum
	cluster.Timeout = 10 * time.Second

	session, err := cluster.CreateSession()
	if err != nil {
		log.Fatalf("Failed to connect to Cassandra: %v", err)
	}
	defer session.Close()

	esURL := os.Getenv("ELASTICSEARCH_URL")
	if esURL == "" {
		esURL = "http://localhost:9200"
	}
	postsIndex := os.Getenv("ELASTICSEARCH_INDEX_POSTS")
	if postsIndex == "" {
		postsIndex = "posts"
	}
	usersIndex := os.Getenv("ELASTICSEARCH_INDEX_USERS")
	if usersIndex == "" {
		usersIndex = "users"
	}

	ctx := context.Background()
	if err := search.EnsureESIndexes(ctx, esURL, postsIndex, usersIndex); err != nil {
		log.Fatalf("Failed to ensure ES indexes: %v", err)
	}

	esClient := search.NewESClient()
	userRepo := data.NewUserRepository(session)

	var rdb *redis.Client
	redisAddr := fmt.Sprintf("%s:%s", os.Getenv("REDIS_HOST"), os.Getenv("REDIS_PORT"))
	if os.Getenv("REDIS_HOST") == "" {
		redisAddr = "localhost:6379"
	}
	rdb = redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("Redis unavailable, skipping username autocomplete sync: %v", err)
		rdb = nil
	}

	iter := session.Query(`
		SELECT post_id, user_id, content, latitude, longitude, geohash, created_at
		FROM posts_by_id
	`).WithContext(ctx).Iter()

	var (
		postID    gocql.UUID
		userID    gocql.UUID
		content   string
		latitude  float64
		longitude float64
		geohash   string
		createdAt time.Time
	)

	indexed := 0
	failed := 0

	for iter.Scan(&postID, &userID, &content, &latitude, &longitude, &geohash, &createdAt) {
		username := ""
		if user, err := userRepo.GetUserByID(ctx, userID.String()); err == nil && user != nil {
			username = user.Username
		}

		event := search.PostCreatedEvent{
			PostID:    postID.String(),
			UserID:    userID.String(),
			Username:  username,
			Content:   content,
			Hashtags:  search.ExtractHashtags(content),
			Lat:       latitude,
			Lon:       longitude,
			Geohash:   geohash,
			CreatedAt: createdAt,
			LikeCount: 0,
		}

		doc := search.PostDocumentFromEvent(event)
		if err := esClient.IndexDocument(ctx, postsIndex, event.PostID, doc); err != nil {
			log.Printf("Failed to index post %s: %v", event.PostID, err)
			failed++
			continue
		}

		if rdb != nil && username != "" {
			_ = rdb.ZAdd(ctx, "users:autocomplete", redis.Z{
				Score:  0,
				Member: username + "\xff",
			}).Err()
		}

		indexed++
	}

	if err := iter.Close(); err != nil {
		log.Fatalf("Failed while scanning posts: %v", err)
	}

	log.Printf("Backfill complete: indexed=%d failed=%d", indexed, failed)
}
