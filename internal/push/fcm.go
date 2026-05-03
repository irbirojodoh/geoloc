package push

import (
	"context"
	"fmt"
	"log/slog"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

// FCMService implements PushService using Firebase Cloud Messaging API v1
type FCMService struct {
	client *messaging.Client
}

// NewFCMService creates a new FCM client
func NewFCMService(ctx context.Context, projectID string, credentialsJSON string) (*FCMService, error) {
	var opts []option.ClientOption
	if credentialsJSON != "" {
		opts = append(opts, option.WithCredentialsJSON([]byte(credentialsJSON)))
	}
	
	config := &firebase.Config{ProjectID: projectID}
	app, err := firebase.NewApp(ctx, config, opts...)
	if err != nil {
		return nil, fmt.Errorf("error initializing app: %v", err)
	}

	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting Messaging client: %v", err)
	}

	slog.Info("FCM service initialized", "project_id", projectID)
	return &FCMService{client: client}, nil
}

func (s *FCMService) Send(ctx context.Context, deviceToken, title, body string, data map[string]string) error {
	message := &messaging.Message{
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data:  data,
		Token: deviceToken,
	}

	response, err := s.client.Send(ctx, message)
	if err != nil {
		return fmt.Errorf("error sending FCM message: %v", err)
	}

	slog.Debug("Successfully sent message", "response", response, "token_prefix", deviceToken[:min(len(deviceToken), 10)])
	return nil
}

func (s *FCMService) SendToUser(ctx context.Context, userID, title, body string, data map[string]string) error {
	// Not used directly in Phase 4 architecture. The consumer reads from DB and sends directly using Send()
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
