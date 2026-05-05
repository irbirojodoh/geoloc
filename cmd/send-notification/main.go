package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gocql/gocql"
	"github.com/joho/godotenv"
	"social-geo-go/internal/data"
	"social-geo-go/internal/notifications/kafka"
)

func main() {
	userID := flag.String("user", "", "User ID to send notification to")
	message := flag.String("message", "This is a manual test notification!", "Notification message")
	title := flag.String("title", "Test Notification", "Notification title (for push)")
	
	flag.Parse()

	if *userID == "" {
		fmt.Println("Usage: go run cmd/send-notification/main.go -user <userID> [-message <msg>] [-title <title>]")
		os.Exit(1)
	}

	// Load env
	if err := godotenv.Load(); err != nil {
		if err := godotenv.Load(".env.development"); err != nil {
			log.Println("No .env file found, relying on environment variables")
		}
	}

	brokersStr := os.Getenv("KAFKA_BROKERS")
	if brokersStr == "" {
		log.Println("KAFKA_BROKERS not set in .env, defaulting to localhost:9092")
		brokersStr = "localhost:9092"
	}
	brokers := strings.Split(brokersStr, ",")

	prefix := os.Getenv("KAFKA_CONSUMER_GROUP_PREFIX")
	if prefix == "" {
		prefix = "geoloc"
	}

	log.Printf("Connecting to Kafka brokers: %v", brokers)
	producer := kafka.NewNotificationEventProducer(brokers)
	defer producer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Send Notification Event (This will trigger DB save + SSE + Push Dispatch if tokens exist)
	event := &kafka.NotificationEvent{
		EventID:     gocql.TimeUUID().String(),
		EventType:   data.NotificationTypeFollow, // using follow as a generic type
		ActorID:     "system",
		RecipientID: *userID,
		TargetType:  "system_test",
		TargetID:    "test-123",
		Message:     *message,
		Payload:     map[string]string{"title": *title, "is_test": "true"},
		CreatedAt:   time.Now().Format(time.RFC3339),
	}

	err := producer.ProduceNotificationEvent(ctx, event)
	if err != nil {
		log.Fatalf("Failed to produce notification event: %v", err)
	}

	log.Printf("✅ Successfully dispatched NotificationEvent for user %s!", *userID)
	log.Println("The notif-persister consumer will now process it, save it to Cassandra, and dispatch a Push job if device tokens exist.")
}
