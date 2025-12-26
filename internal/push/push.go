package push

import (
	"context"
	"log"
)

// PushService defines the interface for push notifications
type PushService interface {
	Send(ctx context.Context, deviceToken, title, body string, data map[string]string) error
	SendToUser(ctx context.Context, userID, title, body string, data map[string]string) error
}

// DeviceToken represents a user's device token
type DeviceToken struct {
	UserID   string `json:"user_id"`
	Token    string `json:"token"`
	Platform string `json:"platform"` // "ios", "android", "web"
}

// LogPushService is a mock implementation that logs notifications
// Replace with FCM/APNs implementation for production
type LogPushService struct {
	tokens map[string][]DeviceToken // userID -> tokens
}

// NewLogPushService creates a new logging push service
func NewLogPushService() *LogPushService {
	return &LogPushService{
		tokens: make(map[string][]DeviceToken),
	}
}

// RegisterDevice registers a device token for a user
func (s *LogPushService) RegisterDevice(userID, token, platform string) {
	s.tokens[userID] = append(s.tokens[userID], DeviceToken{
		UserID:   userID,
		Token:    token,
		Platform: platform,
	})
	log.Printf("[PUSH] Registered device for user %s: %s (%s)", userID, token[:20]+"...", platform)
}

// UnregisterDevice removes a device token
func (s *LogPushService) UnregisterDevice(userID, token string) {
	tokens := s.tokens[userID]
	var filtered []DeviceToken
	for _, t := range tokens {
		if t.Token != token {
			filtered = append(filtered, t)
		}
	}
	s.tokens[userID] = filtered
}

// Send sends a push notification to a specific device
func (s *LogPushService) Send(ctx context.Context, deviceToken, title, body string, data map[string]string) error {
	// In production, implement actual FCM/APNs calls here
	log.Printf("[PUSH] Sending to device: %s - Title: %s, Body: %s", deviceToken[:20]+"...", title, body)
	return nil
}

// SendToUser sends a push notification to all of a user's devices
func (s *LogPushService) SendToUser(ctx context.Context, userID, title, body string, data map[string]string) error {
	tokens := s.tokens[userID]
	if len(tokens) == 0 {
		log.Printf("[PUSH] No devices registered for user %s", userID)
		return nil
	}

	for _, token := range tokens {
		if err := s.Send(ctx, token.Token, title, body, data); err != nil {
			log.Printf("[PUSH] Failed to send to device: %v", err)
		}
	}
	return nil
}

// GetUserTokens returns all device tokens for a user
func (s *LogPushService) GetUserTokens(userID string) []DeviceToken {
	return s.tokens[userID]
}

// FCMService template for Firebase Cloud Messaging
// Uncomment and configure when ready for production
/*
type FCMService struct {
	serverKey string
	client    *http.Client
}

func NewFCMService(serverKey string) *FCMService {
	return &FCMService{
		serverKey: serverKey,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *FCMService) Send(ctx context.Context, deviceToken, title, body string, data map[string]string) error {
	payload := map[string]interface{}{
		"to": deviceToken,
		"notification": map[string]string{
			"title": title,
			"body":  body,
		},
		"data": data,
	}

	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://fcm.googleapis.com/fcm/send", bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "key="+s.serverKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("FCM returned status %d", resp.StatusCode)
	}
	return nil
}
*/
