package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gocql/gocql"

	"social-geo-go/internal/data"
	"social-geo-go/internal/geocoding"
)

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

	// Initialize geocoding client
	geoClient := geocoding.NewNominatimClient("Geoloc/1.0 (backfill)")
	locRepo := data.NewLocationRepository(session, geoClient)
	ctx := context.Background()

	// Get all unique geohash prefixes from posts
	log.Println("Fetching unique geohash prefixes from posts...")

	iter := session.Query(`SELECT DISTINCT geohash_prefix FROM posts_by_geohash`).Iter()

	var geohashPrefixes []string
	var prefix string
	for iter.Scan(&prefix) {
		geohashPrefixes = append(geohashPrefixes, prefix)
	}
	if err := iter.Close(); err != nil {
		log.Fatalf("Failed to fetch geohashes: %v", err)
	}

	log.Printf("Found %d unique geohash prefixes", len(geohashPrefixes))

	// For each geohash, get a sample coordinate and geocode it
	for i, ghPrefix := range geohashPrefixes {
		// Check if already cached
		existing, _ := locRepo.GetByGeohash(ctx, ghPrefix)
		if existing != nil {
			log.Printf("[%d/%d] %s: Already cached (%s, %s)", i+1, len(geohashPrefixes), ghPrefix, existing.Address.Village, existing.Address.City)
			continue
		}

		// Get sample coordinates from this geohash
		var lat, lng float64
		err := session.Query(`
			SELECT latitude, longitude FROM posts_by_geohash WHERE geohash_prefix = ? LIMIT 1
		`, ghPrefix).Scan(&lat, &lng)
		if err != nil {
			log.Printf("[%d/%d] %s: Failed to get coordinates: %v", i+1, len(geohashPrefixes), ghPrefix, err)
			continue
		}

		// Geocode and cache
		log.Printf("[%d/%d] %s: Geocoding (%.4f, %.4f)...", i+1, len(geohashPrefixes), ghPrefix, lat, lng)

		loc, err := locRepo.GetOrFetch(ctx, ghPrefix, lat, lng)
		if err != nil {
			log.Printf("[%d/%d] %s: Geocoding failed: %v", i+1, len(geohashPrefixes), ghPrefix, err)
			continue
		}
		if loc != nil {
			log.Printf("[%d/%d] %s: Cached as '%s, %s, %s'", i+1, len(geohashPrefixes), ghPrefix, loc.Address.Village, loc.Address.City, loc.Address.Country)
		}
	}

	log.Println("✅ Backfill complete!")

	// Verify
	var count int
	session.Query(`SELECT COUNT(*) FROM location_names`).Scan(&count)
	fmt.Printf("📊 Total cached locations: %d\n", count)
}
