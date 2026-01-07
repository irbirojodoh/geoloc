package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/gocql/gocql"
)

var profilePictureURLs = []string{
	"https://shared.irphotoarts.cloud/about/profil-1.jpeg",
	"https://shared.irphotoarts.cloud/about/profil-2.jpeg",
	"https://shared.irphotoarts.cloud/about/profil-3.jpeg",
	"https://shared.irphotoarts.cloud/about/profil-4.jpeg",
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

	// Get all users
	log.Println("Fetching all users...")
	iter := session.Query(`SELECT id, username FROM users`).Iter()

	type UserInfo struct {
		ID       gocql.UUID
		Username string
	}

	var users []UserInfo
	var userID gocql.UUID
	var username string
	for iter.Scan(&userID, &username) {
		users = append(users, UserInfo{ID: userID, Username: username})
	}
	if err := iter.Close(); err != nil {
		log.Fatalf("Failed to fetch users: %v", err)
	}

	log.Printf("Found %d users to update", len(users))

	updated := 0
	for i, user := range users {
		// Pick random profile picture
		randomPic := profilePictureURLs[rand.Intn(len(profilePictureURLs))]

		// Update user profile picture
		err = session.Query(`
			UPDATE users SET profile_picture_url = ? WHERE id = ?
		`, randomPic, user.ID).Exec()
		if err != nil {
			log.Printf("[%d/%d] User %s: Failed to update: %v", i+1, len(users), user.Username, err)
			continue
		}

		log.Printf("[%d/%d] User %s: Set profile picture", i+1, len(users), user.Username)
		updated++
	}

	log.Println("✅ Profile picture backfill complete!")
	fmt.Printf("📊 Updated %d users with profile pictures\n", updated)
}
