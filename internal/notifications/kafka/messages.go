package kafka

// NotificationEvent is the Kafka message payload for notification.events
type NotificationEvent struct {
	EventID     string            `json:"event_id"`
	EventType   string            `json:"event_type"`
	ActorID     string            `json:"actor_id"`
	RecipientID string            `json:"recipient_id"`
	TargetID    string            `json:"target_id,omitempty"`
	TargetType  string            `json:"target_type,omitempty"`
	Message     string            `json:"message"`
	Payload     map[string]string `json:"payload,omitempty"`
	Geohash     string            `json:"geohash,omitempty"`
	CreatedAt   string            `json:"created_at"`
}

// PushDispatchJob is the Kafka message payload for notification.push.dispatch
type PushDispatchJob struct {
	EventID      string            `json:"event_id"`
	UserID       string            `json:"user_id"`
	DeviceTokens []string          `json:"device_tokens"`
	Title        string            `json:"title"`
	Body         string            `json:"body"`
	Data         map[string]string `json:"data"`
	BadgeCount   int               `json:"badge_count"`
	RetryCount   int               `json:"retry_count"`
	DeliverAfter string            `json:"deliver_after,omitempty"`
}

// PushRetryJob extends PushDispatchJob with error context
type PushRetryJob struct {
	PushDispatchJob
	LastError  string `json:"last_error"`
	RetryAfter string `json:"retry_after"`
	MaxRetries int    `json:"max_retries"`
}

// NearbyFanoutJob is for notification.nearby.fanout
type NearbyFanoutJob struct {
	EventID   string `json:"event_id"`
	PostID    string `json:"post_id"`
	AuthorID  string `json:"author_id"`
	Geohash   string `json:"geohash"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}
