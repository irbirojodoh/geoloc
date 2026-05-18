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
	followRepo := data.NewFollowRepository(session)

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

	postsIndexed, postsFailed := backfillPosts(ctx, session, esClient, userRepo, rdb, postsIndex)
	usersIndexed, usersFailed := backfillUsers(ctx, session, esClient, followRepo, rdb, usersIndex)

	log.Printf("Backfill complete: posts indexed=%d failed=%d; users indexed=%d failed=%d",
		postsIndexed, postsFailed, usersIndexed, usersFailed)
}

func backfillPosts(
	ctx context.Context,
	session *gocql.Session,
	esClient *search.ESClient,
	userRepo *data.UserRepository,
	rdb *redis.Client,
	postsIndex string,
) (indexed, failed int) {
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
	return indexed, failed
}

func backfillUsers(
	ctx context.Context,
	session *gocql.Session,
	esClient *search.ESClient,
	followRepo *data.FollowRepository,
	rdb *redis.Client,
	usersIndex string,
) (indexed, failed int) {
	iter := session.Query(`
		SELECT id, username, full_name, profile_picture_url, is_deleted
		FROM users
	`).WithContext(ctx).Iter()

	var (
		userID            gocql.UUID
		username          string
		fullName          string
		profilePictureURL string
		isDeleted         bool
	)

	for iter.Scan(&userID, &username, &fullName, &profilePictureURL, &isDeleted) {
		if isDeleted || username == "" {
			continue
		}

		followerCount := 0
		if counts, err := followRepo.GetFollowCounts(ctx, userID.String()); err == nil && counts != nil {
			followerCount = int(counts.FollowersCount)
		}

		user := &data.User{
			ID:                userID.String(),
			Username:          username,
			FullName:          fullName,
			ProfilePictureURL: profilePictureURL,
		}
		event := search.UserIndexedEventFromUser(user, followerCount)

		if err := search.IndexUserFromEvent(ctx, esClient, rdb, usersIndex, event); err != nil {
			log.Printf("Failed to index user %s: %v", event.UserID, err)
			failed++
			continue
		}
		indexed++
	}

	if err := iter.Close(); err != nil {
		log.Fatalf("Failed while scanning users: %v", err)
	}
	return indexed, failed
}
