package main

import (
	"log"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gocql/gocql"
	"github.com/joho/godotenv"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
	"social-geo-go/internal/handlers"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Cassandra connection
	cluster := gocql.NewCluster(getEnv("CASSANDRA_HOST", "localhost"))
	cluster.Port = 9042
	cluster.Keyspace = getEnv("CASSANDRA_KEYSPACE", "geoloc")
	cluster.Consistency = gocql.Quorum
	cluster.Timeout = 10 * time.Second
	cluster.ConnectTimeout = 10 * time.Second

	// Retry connection with backoff
	var session *gocql.Session
	var err error
	for i := 0; i < 5; i++ {
		session, err = cluster.CreateSession()
		if err == nil {
			break
		}
		log.Printf("Failed to connect to Cassandra (attempt %d/5): %v", i+1, err)
		time.Sleep(time.Duration(i+1) * 2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Unable to connect to Cassandra after 5 attempts: %v", err)
	}
	defer session.Close()
	log.Println("Successfully connected to Cassandra")

	// Initialize repositories
	postRepo := data.NewPostRepository(session)
	userRepo := data.NewUserRepository(session)

	// Setup Gin router
	router := gin.Default()

	// CORS configuration
	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"*"}
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Authorization"}
	router.Use(cors.New(config))

	// Health check
	router.GET("/health", func(c *gin.Context) {
		if err := session.Query("SELECT now() FROM system.local").Exec(); err != nil {
			c.JSON(500, gin.H{"status": "unhealthy", "error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"status": "ok", "database": "cassandra"})
	})

	// ============== PUBLIC ROUTES ==============
	// Auth routes (no authentication required)
	router.POST("/auth/register", handlers.Register(userRepo))
	router.POST("/auth/login", handlers.Login(userRepo))
	router.POST("/auth/refresh", handlers.Refresh)

	// Public feed (read-only)
	router.GET("/api/v1/feed", handlers.GetFeed(postRepo))

	// ============== PROTECTED ROUTES ==============
	// Routes that require authentication
	api := router.Group("/api/v1")
	api.Use(auth.AuthRequired())
	{
		// User routes
		api.GET("/users/:id", handlers.GetUser(userRepo))
		api.GET("/users/username/:username", handlers.GetUserByUsername(userRepo))
		api.GET("/users/:id/posts", handlers.GetUserPosts(postRepo))

		// Post routes (authenticated)
		api.POST("/posts", handlers.CreatePost(postRepo, userRepo))
		api.GET("/posts/:id", handlers.GetPost(postRepo))
	}

	// Start server
	port := getEnv("PORT", "8080")
	log.Printf("Server starting on port %s\n", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v\n", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
