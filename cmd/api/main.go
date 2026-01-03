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
	"social-geo-go/internal/middleware"
	"social-geo-go/internal/push"
	"social-geo-go/internal/storage"
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
	for i := range 5 {
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

	// Initialize storage
	uploadPath := getEnv("UPLOAD_PATH", "./uploads")
	baseURL := getEnv("BASE_URL", "http://localhost:8080")
	store := storage.NewLocalStorage(uploadPath, baseURL+"/uploads")

	// Initialize push service
	pushService := push.NewLogPushService()

	// Initialize repositories
	postRepo := data.NewPostRepository(session)
	userRepo := data.NewUserRepository(session)
	likeRepo := data.NewLikeRepository(session)
	commentRepo := data.NewCommentRepository(session)
	followRepo := data.NewFollowRepository(session)
	locFollowRepo := data.NewLocationFollowRepository(session)
	notifRepo := data.NewNotificationRepository(session)

	// Setup Gin router
	router := gin.Default()

	// Global rate limiter (100 requests per minute per IP)
	router.Use(middleware.RateLimitByIP(100, time.Minute))

	// CORS configuration
	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"*"}
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Authorization"}
	router.Use(cors.New(config))

	// Serve uploaded files
	router.Static("/uploads", uploadPath)

	// Health check
	router.GET("/health", func(c *gin.Context) {
		if err := session.Query("SELECT now() FROM system.local").Exec(); err != nil {
			c.JSON(500, gin.H{"status": "unhealthy", "error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"status": "ok", "database": "cassandra"})
	})

	// ============== PUBLIC ROUTES ==============
	router.POST("/auth/register", handlers.Register(userRepo))
	router.POST("/auth/login", handlers.Login(userRepo))
	router.POST("/auth/refresh", handlers.Refresh)
	router.GET("/api/v1/feed", handlers.GetFeed(postRepo))

	// ============== PROTECTED ROUTES ==============
	api := router.Group("/api/v1")
	api.Use(auth.AuthRequired())
	{
		// Profile
		api.PUT("/users/me", handlers.UpdateProfile(userRepo))

		// User routes
		api.GET("/users/:id", handlers.GetUser(userRepo))
		api.GET("/users/username/:username", handlers.GetUserByUsername(userRepo))
		api.GET("/users/:id/posts", handlers.GetUserPosts(postRepo))

		// Follow routes
		api.POST("/users/:id/follow", handlers.FollowUser(followRepo, notifRepo))
		api.DELETE("/users/:id/follow", handlers.UnfollowUser(followRepo))
		api.GET("/users/:id/followers", handlers.GetFollowers(followRepo))
		api.GET("/users/:id/following", handlers.GetFollowing(followRepo))

		// Post routes
		api.POST("/posts", handlers.CreatePost(postRepo, userRepo))
		api.GET("/posts/:id", handlers.GetPost(postRepo))

		// Post likes
		api.POST("/posts/:id/like", handlers.LikePost(likeRepo))
		api.DELETE("/posts/:id/like", handlers.UnlikePost(likeRepo))

		// Post comments
		api.POST("/posts/:id/comments", handlers.CreateComment(commentRepo))
		api.GET("/posts/:id/comments", handlers.GetComments(commentRepo))

		// Comment routes
		api.POST("/comments/:id/reply", handlers.ReplyToComment(commentRepo))
		api.POST("/comments/:id/like", handlers.LikeComment(likeRepo))
		api.DELETE("/comments/:id/like", handlers.UnlikeComment(likeRepo))
		api.DELETE("/comments/:id", handlers.DeleteComment(commentRepo))

		// Location follow routes
		api.POST("/locations/follow", handlers.FollowLocation(locFollowRepo))
		api.DELETE("/locations/:geohash/follow", handlers.UnfollowLocation(locFollowRepo))
		api.GET("/locations/following", handlers.GetFollowedLocations(locFollowRepo))

		// Notification routes
		api.GET("/notifications", handlers.GetNotifications(notifRepo))
		api.PUT("/notifications/:id/read", handlers.MarkNotificationAsRead(notifRepo))
		api.PUT("/notifications/read-all", handlers.MarkAllNotificationsAsRead(notifRepo))

		// Search routes
		api.GET("/search/users", handlers.SearchUsers(userRepo))
		api.GET("/search/posts", handlers.SearchPosts(postRepo))

		// Upload routes
		api.POST("/upload/avatar", handlers.UploadAvatar(store))
		api.POST("/upload/post", handlers.UploadPostMedia(store))

		// Device registration (push notifications)
		api.POST("/devices", handlers.RegisterDevice(pushService))
		api.DELETE("/devices", handlers.UnregisterDevice(pushService))
	}

	// Start server
	port := getEnv("PORT", "8080")
	log.Printf("Server starting on port %s\n", port)
	log.Printf("Uploads directory: %s\n", uploadPath)
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
