package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gocql/gocql"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/cache"
	"social-geo-go/internal/data"
	"social-geo-go/internal/geocoding"
	"social-geo-go/internal/handlers"
	"social-geo-go/internal/middleware"
	"social-geo-go/internal/notifications"
	"social-geo-go/internal/notifications/kafka"
	"social-geo-go/internal/notifications/sse"
	"social-geo-go/internal/push"
	"social-geo-go/internal/search"
	"social-geo-go/internal/storage"

	"github.com/lmittmann/tint"
)

func main() {
	// Load environment variables (env-specific file first, then .env fallback)
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
	slog.Info("Environment loaded", "APP_ENV", appEnv)

	slog.SetDefault(slog.New(
		tint.NewHandler(os.Stdout, &tint.Options{
			Level: slog.LevelDebug,
		}),
	))

	// Initialize OAuth Providers
	auth.InitOAuth()

	// Retry connection with backoff
	var session *gocql.Session
	var err error
	for i := range 5 {
		// Cassandra connection config must be recreated per attempt
		// as gocql modifies the HostSelectionPolicy internally during CreateSession
		cluster := gocql.NewCluster(getEnv("CASSANDRA_HOST", "localhost"))

		// Parse port from env or default to 9042
		portStr := getEnv("CASSANDRA_PORT", "9042")
		port, errPort := strconv.Atoi(portStr)
		if errPort != nil {
			log.Fatalf("Invalid CASSANDRA_PORT: %s", portStr)
		}
		cluster.Port = port

		cluster.Keyspace = getEnv("CASSANDRA_KEYSPACE", "geoloc")
		cluster.Consistency = gocql.Quorum
		cluster.Timeout = 10 * time.Second
		cluster.ConnectTimeout = 10 * time.Second
		cluster.NumConns = 4 // Connection Pooling limit
		cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(gocql.RoundRobinHostPolicy())

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

	// Initialize Redis connection
	var likeCounter *cache.LikeCounter
	var commentCounter *cache.CommentCounter
	redisClient, err := cache.NewRedisClient()
	if err != nil {
		log.Printf("WARNING: Failed to connect to Redis: %v", err)
		log.Println("Like/Comment counters will use Cassandra fallback (slower)")
	} else {
		defer redisClient.Close()
		log.Println("Successfully connected to Redis")
		likeCounter = cache.NewLikeCounter(redisClient)
		commentCounter = cache.NewCommentCounter(redisClient)
	}

	// Initialize storage
	uploadPath := getEnv("UPLOAD_PATH", "./uploads")
	baseURL := getEnv("BASE_URL", "http://localhost:8080")
	store := storage.NewLocalStorage(uploadPath, baseURL+"/uploads")

	// Initialize push service
	deviceRepo := data.NewDeviceRepository(session)

	// Initialize geocoding client
	geoClient := geocoding.NewNominatimClient("Geoloc/1.0 (dev@geoloc.app)")

	// Initialize repositories
	postRepo := data.NewPostRepository(session)
	userRepo := data.NewUserRepository(session)
	likeRepo := data.NewLikeRepository(session, likeCounter)
	commentRepo := data.NewCommentRepository(session, commentCounter)
	followRepo := data.NewFollowRepository(session)
	locFollowRepo := data.NewLocationFollowRepository(session)

	var rawRedisClient *redis.Client
	if redisClient != nil {
		rawRedisClient = redisClient.Client()
	}
	notifRepo := data.NewNotificationRepository(session, rawRedisClient)

	// Initialize Notification Producer & Dispatcher
	var notifProducer kafka.NotificationEventProducer
	if os.Getenv("KAFKA_NOTIFICATIONS_ENABLED") == "true" {
		brokers := strings.Split(os.Getenv("KAFKA_BROKERS"), ",")
		if len(brokers) > 0 && brokers[0] != "" {
			notifProducer = kafka.NewNotificationEventProducer(brokers)
			defer notifProducer.Close()
			log.Println("Kafka Notification Producer enabled")
		}
	}
	notifDispatcher := notifications.NewDispatcher(notifProducer, notifRepo, rawRedisClient)

	var searchIndexer search.SearchIndexer
	if brokersStr := os.Getenv("KAFKA_BROKERS"); brokersStr != "" {
		brokers := strings.Split(brokersStr, ",")
		if len(brokers) > 0 && brokers[0] != "" {
			searchIndexer = search.NewSearchIndexer(brokers)
			defer searchIndexer.Close()
			log.Println("Kafka Search Indexer Producer enabled")
		}
	}

	locRepo := data.NewLocationRepository(session, geoClient)
	resetRepo := data.NewPasswordResetRepository(session)
	modRepo := data.NewModerationRepository(session)

	// Initialize Elasticsearch and search service
	esClient := search.NewESClient()
	searchSvc := search.NewService(esClient, rawRedisClient)
	searchHandler := handlers.NewNewSearchHandler(searchSvc, session, userRepo, locRepo, likeRepo)

	// Setup Gin router
	router := gin.Default()

	// Global rate limiter (100 requests per minute per IP) backed by Redis
	router.Use(middleware.RateLimitByIP(redisClient, 100, time.Minute))

	// Global request timeout (10 seconds) to prevent frozen external calls
	router.Use(middleware.TimeoutMiddleware(10 * time.Second))

	// CORS configuration — restrict to allowed origins
	allowedOrigins := getEnv("ALLOWED_ORIGINS", "http://localhost:3000")
	config := cors.DefaultConfig()
	config.AllowOrigins = strings.Split(allowedOrigins, ",")
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Authorization"}
	router.Use(cors.New(config))
	slog.Info("CORS configured", "allowed_origins", config.AllowOrigins)

	// Serve uploaded files
	router.Static("/uploads", uploadPath)

	// Health check (checks Cassandra + Redis)
	router.GET("/health", func(c *gin.Context) {
		health := gin.H{"status": "ok"}

		// Check Cassandra
		if err := session.Query("SELECT now() FROM system.local").Exec(); err != nil {
			health["status"] = "degraded"
			health["cassandra"] = "unhealthy"
		} else {
			health["cassandra"] = "ok"
		}

		// Check Redis
		if redisClient != nil {
			if err := redisClient.Ping(c.Request.Context()); err != nil {
				health["status"] = "degraded"
				health["redis"] = "unhealthy"
			} else {
				health["redis"] = "ok"
			}
		} else {
			health["redis"] = "not configured"
		}

		status := http.StatusOK
		if health["status"] != "ok" {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, health)
	})

	// ============== PUBLIC ROUTES ==============
	router.POST("/auth/register", handlers.Register(userRepo, searchIndexer))
	router.POST("/auth/login", handlers.Login(userRepo))

	// Mobile-native social login: Flutter app verifies natively and sends ID token here
	router.POST("/auth/google/token", handlers.GoogleLogin(userRepo, searchIndexer))
	router.POST("/auth/apple/token", handlers.AppleLogin(userRepo, searchIndexer))

	// Web-based OAuth redirect flow (kept for browser/web compatibility)
	router.GET("/auth/:provider/login", handlers.LoginOAuth())
	router.GET("/auth/:provider/callback", handlers.CompleteOAuth(userRepo, searchIndexer))
	router.POST("/auth/:provider/callback", handlers.CompleteOAuth(userRepo, searchIndexer)) // Apple uses POST
	router.POST("/auth/refresh", handlers.Refresh)

	// Password reset (public)
	router.POST("/auth/forgot-password", handlers.ForgotPassword(userRepo, resetRepo))
	router.POST("/auth/reset-password", handlers.ResetPassword(userRepo, resetRepo))

	// Readiness probe (no dependency checks — server is up and accepting traffic)
	router.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	// ============== PROTECTED ROUTES ==============
	api := router.Group("/api/v1")
	api.Use(auth.AuthRequired())
	{
		// Feed (now protected — filters blocked/muted users)
		api.GET("/feed", handlers.GetFeed(postRepo, userRepo, locRepo, likeRepo, modRepo))

		// Geocode
		api.GET("/geocode/address", handlers.GetAddress(locRepo))

		// Profile
		api.GET("/users/me", handlers.GetCurrentUser(userRepo))
		api.PUT("/users/me", handlers.UpdateProfile(userRepo, followRepo, searchIndexer))
		api.DELETE("/users/me", handlers.DeleteAccount(userRepo))

		// User routes
		api.GET("/users/:id", handlers.GetUser(userRepo))
		api.GET("/users/username/:username", handlers.GetUserByUsername(userRepo))
		api.GET("/users/:id/posts", handlers.GetUserPosts(postRepo, userRepo, locRepo, likeRepo))
		api.GET("/users/:id/liked-posts", handlers.GetLikedPosts(likeRepo, postRepo, userRepo, locRepo))

		// Follow routes
		api.POST("/users/:id/follow", handlers.FollowUser(followRepo, notifDispatcher))
		api.DELETE("/users/:id/follow", handlers.UnfollowUser(followRepo))
		api.GET("/users/:id/followers", handlers.GetFollowers(followRepo))
		api.GET("/users/:id/following", handlers.GetFollowing(followRepo))

		// Block/Mute routes
		api.POST("/users/:id/block", handlers.BlockUser(modRepo))
		api.DELETE("/users/:id/block", handlers.UnblockUser(modRepo))
		api.POST("/users/:id/mute", handlers.MuteUser(modRepo))
		api.DELETE("/users/:id/mute", handlers.UnmuteUser(modRepo))
		api.GET("/users/me/blocked", handlers.GetBlockedUsers(modRepo))
		api.GET("/users/me/muted", handlers.GetMutedUsers(modRepo))

		// Post routes
		api.POST("/posts", handlers.CreatePost(postRepo, userRepo, notifDispatcher, searchIndexer))
		api.GET("/posts/:id", handlers.GetPost(postRepo, userRepo, locRepo, likeRepo))
		api.DELETE("/posts/:id", handlers.DeletePost(postRepo))

		// Post likes (legacy + new idempotent toggle)
		api.POST("/posts/:id/like", handlers.LikePost(likeRepo))
		api.DELETE("/posts/:id/like", handlers.UnlikePost(likeRepo))
		api.POST("/posts/:id/toggle-like", handlers.TogglePostLike(likeRepo, postRepo, notifDispatcher))

		// Post comments
		api.POST("/posts/:id/comments", handlers.CreateComment(commentRepo, postRepo, notifDispatcher))
		api.GET("/posts/:id/comments", handlers.GetComments(commentRepo, userRepo, likeRepo))

		// Comment routes
		api.POST("/comments/:id/reply", handlers.ReplyToComment(commentRepo, notifDispatcher))
		api.GET("/comments/:id/replies", handlers.GetReplies(commentRepo, userRepo, likeRepo))
		api.PUT("/comments/:id", handlers.EditComment(commentRepo))
		api.DELETE("/comments/:id", handlers.DeleteComment(commentRepo))
		api.POST("/comments/:id/like", handlers.LikeComment(likeRepo))
		api.DELETE("/comments/:id/like", handlers.UnlikeComment(likeRepo))
		api.POST("/comments/:id/toggle-like", handlers.ToggleCommentLike(likeRepo, commentRepo, notifDispatcher))

		// Location follow routes
		api.POST("/locations/follow", handlers.FollowLocation(locFollowRepo))
		api.DELETE("/locations/:geohash/follow", handlers.UnfollowLocation(locFollowRepo))
		api.GET("/locations/following", handlers.GetFollowedLocations(locFollowRepo))

		// Notification routes
		api.GET("/notifications", handlers.GetNotifications(notifRepo))
		api.GET("/notifications/stream", sse.StreamNotifications(rawRedisClient))
		api.GET("/notifications/unread-count", handlers.GetUnreadCount(notifRepo))
		api.PUT("/notifications/:id/read", handlers.MarkNotificationAsRead(notifRepo))
		api.PUT("/notifications/read-all", handlers.MarkAllNotificationsAsRead(notifRepo))
		api.DELETE("/notifications/:id", handlers.DeleteNotification(notifRepo))

		// Search routes (legacy Cassandra-backed)
		api.GET("/search/users", handlers.SearchUsers(userRepo))
		api.GET("/search/posts", handlers.SearchPosts(postRepo, userRepo, likeRepo))

		// Search routes (Elasticsearch-backed)
		api.GET("/search/nearby", searchHandler.SearchNearbyHandler)
		api.GET("/search", searchHandler.SearchHandler)
		api.GET("/autocomplete", searchHandler.AutocompleteHandler)

		// Upload routes
		api.POST("/upload/avatar", handlers.UploadAvatar(store))
		api.POST("/upload/post", handlers.UploadPostMedia(store))

		// Device registration (push notifications)
		api.POST("/devices", handlers.RegisterDevice(deviceRepo))
		api.DELETE("/devices", handlers.UnregisterDevice(deviceRepo))

		// Content moderation
		api.POST("/reports", handlers.CreateReport(modRepo))
	}

	// Start Kafka Consumers
	var consumerCtx context.Context
	var consumerCancel context.CancelFunc
	if os.Getenv("KAFKA_NOTIFICATIONS_ENABLED") == "true" {
		consumerCtx, consumerCancel = context.WithCancel(context.Background())
		brokers := strings.Split(os.Getenv("KAFKA_BROKERS"), ",")
		prefix := os.Getenv("KAFKA_CONSUMER_GROUP_PREFIX")
		if prefix == "" {
			prefix = "geoloc"
		}

		if len(brokers) > 0 && brokers[0] != "" {
			persisterHandler := kafka.NewPersisterHandler(notifRepo, rawRedisClient, modRepo, deviceRepo, notifProducer)
			go kafka.RunConsumerGroup(consumerCtx, brokers, prefix+"-notif-persister", "notification.events", persisterHandler.Handle)
			log.Println("Started notif-persister consumer group")

			if rawRedisClient != nil {
				sseHandler := kafka.NewSSEFanoutHandler(rawRedisClient)
				go kafka.RunConsumerGroup(consumerCtx, brokers, prefix+"-notif-sse-fanout", "notification.events", sseHandler.Handle)
				log.Println("Started notif-sse-fanout consumer group")
			}

			// Push Notifications Service
			var pushService push.PushService
			if os.Getenv("PUSH_NOTIFICATIONS_ENABLED") == "true" && os.Getenv("FCM_PROJECT_ID") != "" {
				fcmSvc, err := push.NewFCMService(context.Background(), os.Getenv("FCM_PROJECT_ID"), os.Getenv("FCM_CREDENTIALS_JSON"))
				if err != nil {
					log.Printf("Failed to init FCM: %v, falling back to mock", err)
					pushService = push.NewLogPushService()
				} else {
					pushService = fcmSvc
				}
			} else {
				pushService = push.NewLogPushService()
			}

			pushDispatchHandler := kafka.NewPushDispatchHandler(pushService, notifProducer)
			go kafka.RunConsumerGroup(consumerCtx, brokers, prefix+"-notif-push-dispatch", "notification.push.dispatch", pushDispatchHandler.Handle)
			log.Println("Started notif-push-dispatch consumer group")

			pushRetryHandler := kafka.NewPushRetryHandler(pushService, notifProducer)
			go kafka.RunConsumerGroup(consumerCtx, brokers, prefix+"-notif-push-retry", "notification.push.retry", pushRetryHandler.Handle)
			log.Println("Started notif-push-retry consumer group")

			nearbyFanoutHandler := kafka.NewNearbyFanoutHandler(locFollowRepo, notifProducer)
			go kafka.RunConsumerGroup(consumerCtx, brokers, prefix+"-notif-nearby-fanout", "notification.nearby.fanout", nearbyFanoutHandler.Handle)
			log.Println("Started notif-nearby-fanout consumer group")
		}
	}

	// Start server
	port := getEnv("PORT", "8080")

	slog.Info("Starting Server", "url", baseURL)
	slog.Info("Uploads directory: ", "upload_path", uploadPath)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Initializing the server in a goroutine so that it won't block the graceful shutdown handling below
	go func() {
		log.Printf("Server starting on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with a timeout of 5 seconds.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	// Cleanup background resources
	if consumerCancel != nil {
		consumerCancel()
	}
	geoClient.Close()
	slog.Info("Server shutdown complete")

	log.Println("Server exiting")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	slog.Warn("[ENV] Undefined variable", "variable", key, "default", defaultValue)
	return defaultValue
}
