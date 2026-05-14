package data

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

func deviceIDFromToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:16])
}

// RegisterDevice saves a device token for a user
func (r *DeviceRepository) RegisterDevice(ctx context.Context, userID, token, platform string) error {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	deviceID := deviceIDFromToken(token)
	now := time.Now()

	batch := r.session.NewBatch(gocql.LoggedBatch).WithContext(ctx)
	batch.Query(`
		INSERT INTO push_device_tokens (user_id, device_id, platform, fcm_token, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, uid, deviceID, platform, token, now, now)
	batch.Query(`
		INSERT INTO device_tokens_by_token (fcm_token, user_id, device_id)
		VALUES (?, ?, ?)
	`, token, uid, deviceID)

	return r.session.ExecuteBatch(batch)
}

// UnregisterDevice removes a device token for a user
func (r *DeviceRepository) UnregisterDevice(ctx context.Context, userID, token string) error {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	deviceID := deviceIDFromToken(token)

	batch := r.session.NewBatch(gocql.LoggedBatch).WithContext(ctx)
	batch.Query(`
		DELETE FROM push_device_tokens WHERE user_id = ? AND device_id = ?
	`, uid, deviceID)
	batch.Query(`
		DELETE FROM device_tokens_by_token WHERE fcm_token = ?
	`, token)

	return r.session.ExecuteBatch(batch)
}

// GetDeviceTokens returns all device tokens for a user
func (r *DeviceRepository) GetDeviceTokens(ctx context.Context, userID string) ([]string, error) {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	query := `
		SELECT fcm_token FROM push_device_tokens WHERE user_id = ?
	`
	iter := r.session.Query(query, uid).WithContext(ctx).Iter()
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
