package main

import (
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/gocql/gocql"
	"github.com/mmcloughlin/geohash"
)

// Test location: Jakarta, Indonesia (06°21′55″S 106°49′37″E)
const (
	baseLat = -6.3653
	baseLng = 106.8269
)

var usernames = []string{
	"john_doe", "jane_smith", "alex_wong", "maria_garcia", "david_chen",
	"sarah_lee", "mike_brown", "emma_wilson", "ryan_taylor", "lisa_anderson",
	"tom_martinez", "nina_patel", "chris_jackson", "amy_moore", "kevin_white",
	"olivia_harris", "james_clark", "sophia_lewis", "daniel_walker", "mia_robinson",
}

var fullNames = []string{
	"John Doe", "Jane Smith", "Alex Wong", "Maria Garcia", "David Chen",
	"Sarah Lee", "Mike Brown", "Emma Wilson", "Ryan Taylor", "Lisa Anderson",
	"Tom Martinez", "Nina Patel", "Chris Jackson", "Amy Moore", "Kevin White",
	"Olivia Harris", "James Clark", "Sophia Lewis", "Daniel Walker", "Mia Robinson",
}

var bios = []string{
	"Tech enthusiast", "Food blogger", "Photographer", "Travel lover", "Coffee addict",
	"Fitness enthusiast", "Music producer", "Book worm", "Startup founder", "Art lover",
	"Gaming streamer", "Yoga instructor", "Sports fan", "Nature explorer", "Movie critic",
	"Fashion designer", "Chef", "Pet lover", "Tech blogger", "Student life",
}

var postContents = []string{
	"Beautiful morning in Jakarta! ☀️",
	"Just discovered this amazing coffee shop nearby",
	"The traffic today is unbelievable 🚗",
	"Love the street food here! 🍜",
	"Perfect weather for a walk in the park",
	"Working from my favorite cafe today",
	"This sunset is absolutely stunning 🌅",
	"Found the best nasi goreng in town!",
	"Meeting up with friends at the mall",
	"Exploring new places in the city",
	"Rainy day vibes ☔",
	"Best bakso I've ever had!",
	"Weekend market finds 🛍️",
	"Morning jog around the neighborhood",
	"This cafe has the best wifi",
	"Late night food run 🌙",
	"Shopping day with the squad",
	"Amazing rooftop view from here",
	"Trying out a new restaurant",
	"City lights at night are beautiful ✨",
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
	rand.Seed(time.Now().UnixNano())

	// Create users and store their IDs
	userIDs := make([]gocql.UUID, len(usernames))
	for i, username := range usernames {
		userID := gocql.TimeUUID()
		userIDs[i] = userID

		err := session.Query(`
			INSERT INTO users (id, username, email, full_name, bio, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			userID, username, username+"@example.com", fullNames[i], bios[i],
			"$sha3$hashed_password", time.Now(), time.Now(),
		).Exec()

		if err != nil {
			log.Printf("Failed to insert user %s: %v", username, err)
		} else {
			log.Printf("Created user: %s (%s)", username, userID)
		}
	}

	// Create 10 posts per user (200 posts total)
	for i, userID := range userIDs {
		for j := 0; j < 10; j++ {
			postID := gocql.TimeUUID()

			// Add small random offset to location (within ~1km)
			lat := baseLat + (rand.Float64()-0.5)*0.02
			lng := baseLng + (rand.Float64()-0.5)*0.02

			fullGeohash := geohash.Encode(lat, lng)
			geohashPrefix := fullGeohash[:5]

			content := postContents[rand.Intn(len(postContents))]
			createdAt := time.Now().Add(-time.Duration(rand.Intn(720)) * time.Hour)

			// Insert into posts_by_geohash
			err := session.Query(`
				INSERT INTO posts_by_geohash (geohash_prefix, created_at, post_id, user_id, content, latitude, longitude, full_geohash)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				geohashPrefix, createdAt, postID, userID, content, lat, lng, fullGeohash,
			).Exec()
			if err != nil {
				log.Printf("Failed to insert post_by_geohash: %v", err)
			}

			// Insert into posts_by_id
			err = session.Query(`
				INSERT INTO posts_by_id (post_id, user_id, content, latitude, longitude, geohash, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				postID, userID, content, lat, lng, fullGeohash, createdAt,
			).Exec()
			if err != nil {
				log.Printf("Failed to insert post_by_id: %v", err)
			}

			// Insert into posts_by_user
			err = session.Query(`
				INSERT INTO posts_by_user (user_id, created_at, post_id, content, latitude, longitude)
				VALUES (?, ?, ?, ?, ?, ?)`,
				userID, createdAt, postID, content, lat, lng,
			).Exec()
			if err != nil {
				log.Printf("Failed to insert post_by_user: %v", err)
			}
		}
		log.Printf("Created 10 posts for user: %s", usernames[i])
	}

	log.Println("✅ Seed data created successfully!")
	log.Printf("📊 Created %d users and %d posts", len(usernames), len(usernames)*10)
	log.Printf("📍 Location: Jakarta, Indonesia (%.4f, %.4f)", baseLat, baseLng)
}
