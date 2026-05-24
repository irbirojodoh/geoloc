package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/gocql/gocql"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
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

	ctx := context.Background()

	cassandraHost := getEnv("CASSANDRA_HOST", "localhost")
	cassandraKeyspace := getEnv("CASSANDRA_KEYSPACE", "geoloc")
	cassandraPort, err := strconv.Atoi(getEnv("CASSANDRA_PORT", "9042"))
	if err != nil {
		log.Fatalf("Invalid CASSANDRA_PORT: %v", err)
	}

	cluster := gocql.NewCluster(cassandraHost)
	cluster.Port = cassandraPort
	cluster.Keyspace = cassandraKeyspace
	cluster.Consistency = gocql.Quorum
	cluster.Timeout = 10 * time.Second
	cluster.ConnectTimeout = 10 * time.Second

	session, err := cluster.CreateSession()
	if err != nil {
		log.Fatalf("Failed to connect to Cassandra: %v", err)
	}
	defer session.Close()

	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")
	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisDB, _ := strconv.Atoi(getEnv("REDIS_DB", "0"))

	rdb := redis.NewClient(&redis.Options{
		Addr:     redisHost + ":" + redisPort,
		Password: redisPassword,
		DB:       redisDB,
	})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	var (
		postID gocql.UUID
		count  int64
	)

	iter := session.Query(`
		SELECT post_id, count
		FROM comment_counts
	`).WithContext(ctx).Iter()

	var scanned, updated, deleted, failed int

	for iter.Scan(&postID, &count) {
		scanned++
		key := "comment_count:" + postID.String()

		if count <= 0 {
			if err := rdb.Del(ctx, key).Err(); err != nil {
				failed++
				log.Printf("Failed deleting %s: %v", key, err)
				continue
			}
			deleted++
			continue
		}

		if err := rdb.Set(ctx, key, count, 0).Err(); err != nil {
			failed++
			log.Printf("Failed setting %s=%d: %v", key, count, err)
			continue
		}
		updated++
	}

	if err := iter.Close(); err != nil {
		log.Fatalf("Failed while scanning comment_counts: %v", err)
	}

	log.Printf("Comment count backfill complete: scanned=%d updated=%d deleted=%d failed=%d", scanned, updated, deleted, failed)
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
