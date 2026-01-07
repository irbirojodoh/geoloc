package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/gocql/gocql"
)

var imageURLs = []string{
	"https://shared.irphotoarts.cloud/about/image%20-%201.jpg",
	"https://shared.irphotoarts.cloud/about/image%20-%202.jpg",
	"https://shared.irphotoarts.cloud/about/image%20-%203.jpeg",
	"https://shared.irphotoarts.cloud/about/image%20-%204.jpeg",
}

func main() {
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

	log.Println("Connected to Cassandra")

	// Seed random
	rand.Seed(time.Now().UnixNano())

	// Get all posts from posts_by_id
	log.Println("Fetching all posts...")
	iter := session.Query(`SELECT post_id FROM posts_by_id`).Iter()

	var postIDs []gocql.UUID
	var postID gocql.UUID
	for iter.Scan(&postID) {
		postIDs = append(postIDs, postID)
	}
	if err := iter.Close(); err != nil {
		log.Fatalf("Failed to fetch posts: %v", err)
	}

	log.Printf("Found %d posts to update", len(postIDs))

	updated := 0
	for i, pid := range postIDs {
		// Random number of images (0-4)
		numImages := rand.Intn(5) // 0, 1, 2, 3, or 4

		if numImages == 0 {
			log.Printf("[%d/%d] Post %s: No images (skipped)", i+1, len(postIDs), pid)
			continue
		}

		// Shuffle and pick random images
		shuffled := make([]string, len(imageURLs))
		copy(shuffled, imageURLs)
		rand.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		selectedImages := shuffled[:numImages]

		// Update posts_by_id
		err = session.Query(`
			UPDATE posts_by_id SET media_urls = ? WHERE post_id = ?
		`, selectedImages, pid).Exec()
		if err != nil {
			log.Printf("[%d/%d] Post %s: Failed to update posts_by_id: %v", i+1, len(postIDs), pid, err)
			continue
		}

		// Get post details for updating other tables
		var userID gocql.UUID
		var geohash string
		var createdAt time.Time
		err = session.Query(`
			SELECT user_id, geohash, created_at FROM posts_by_id WHERE post_id = ?
		`, pid).Scan(&userID, &geohash, &createdAt)
		if err != nil {
			log.Printf("[%d/%d] Post %s: Failed to get post details: %v", i+1, len(postIDs), pid, err)
			continue
		}

		// Update posts_by_geohash
		geohashPrefix := geohash
		if len(geohash) >= 5 {
			geohashPrefix = geohash[:5]
		}
		session.Query(`
			UPDATE posts_by_geohash SET media_urls = ? WHERE geohash_prefix = ? AND created_at = ? AND post_id = ?
		`, selectedImages, geohashPrefix, createdAt, pid).Exec()

		// Update posts_by_user
		session.Query(`
			UPDATE posts_by_user SET media_urls = ? WHERE user_id = ? AND created_at = ? AND post_id = ?
		`, selectedImages, userID, createdAt, pid).Exec()

		log.Printf("[%d/%d] Post %s: Added %d images", i+1, len(postIDs), pid, numImages)
		updated++
	}

	log.Println("✅ Backfill complete!")
	fmt.Printf("📊 Updated %d posts with images\n", updated)
}
