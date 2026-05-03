package data

import (
	"context"
	"time"

	"github.com/gocql/gocql"
)

// DeviceRepository manages push notification device tokens
type DeviceRepository struct {
	session *gocql.Session
}

func NewDeviceRepository(session *gocql.Session) *DeviceRepository {
	return &DeviceRepository{session: session}
}

// RegisterDevice saves a device token for a user
func (r *DeviceRepository) RegisterDevice(ctx context.Context, userID, token, platform string) error {
	query := `
		INSERT INTO push_device_tokens (user_id, device_token, platform, last_used_at)
		VALUES (?, ?, ?, ?)
	`
	return r.session.Query(query, userID, token, platform, time.Now()).WithContext(ctx).Exec()
}

// UnregisterDevice removes a device token for a user
func (r *DeviceRepository) UnregisterDevice(ctx context.Context, userID, token string) error {
	query := `
		DELETE FROM push_device_tokens WHERE user_id = ? AND device_token = ?
	`
	return r.session.Query(query, userID, token).WithContext(ctx).Exec()
}

// GetDeviceTokens returns all device tokens for a user
func (r *DeviceRepository) GetDeviceTokens(ctx context.Context, userID string) ([]string, error) {
	query := `
		SELECT device_token FROM push_device_tokens WHERE user_id = ?
	`
	iter := r.session.Query(query, userID).WithContext(ctx).Iter()
	var tokens []string
	var token string
	for iter.Scan(&token) {
		tokens = append(tokens, token)
	}
	if err := iter.Close(); err != nil {
		return nil, err
	}
	return tokens, nil
}
