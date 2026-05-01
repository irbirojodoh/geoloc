package data

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gocql/gocql"
)

// PasswordResetRepository handles password reset token storage
type PasswordResetRepository struct {
	session *gocql.Session
}

// NewPasswordResetRepository creates a new password reset repository
func NewPasswordResetRepository(session *gocql.Session) *PasswordResetRepository {
	return &PasswordResetRepository{session: session}
}

const (
	// ResetTokenTTL is the lifetime of a password reset token
	ResetTokenTTL = 1 * time.Hour
	// ResetTokenLength is the number of random bytes (hex-encoded = 64 chars)
	ResetTokenLength = 32
)

// CreateToken generates and stores a new password reset token for a user
func (r *PasswordResetRepository) CreateToken(ctx context.Context, userID string) (string, error) {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return "", fmt.Errorf("invalid user_id: %w", err)
	}

	// Generate cryptographically secure random token
	tokenBytes := make([]byte, ResetTokenLength)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	now := time.Now()
	expiresAt := now.Add(ResetTokenTTL)

	err = r.session.Query(`
		INSERT INTO password_reset_tokens (reset_token, user_id, expires_at, used, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, token, uid, expiresAt, false, now).WithContext(ctx).Exec()

	if err != nil {
		return "", fmt.Errorf("failed to store reset token: %w", err)
	}

	return token, nil
}

// ValidateToken checks if a token is valid (exists, not expired, not used) and returns the user ID
func (r *PasswordResetRepository) ValidateToken(ctx context.Context, token string) (string, error) {
	var userID gocql.UUID
	var expiresAt time.Time
	var used bool

	err := r.session.Query(`
		SELECT user_id, expires_at, used FROM password_reset_tokens WHERE reset_token = ?
	`, token).WithContext(ctx).Scan(&userID, &expiresAt, &used)

	if err != nil {
		if err == gocql.ErrNotFound {
			return "", fmt.Errorf("invalid or expired reset token")
		}
		return "", fmt.Errorf("failed to validate token: %w", err)
	}

	if used {
		return "", fmt.Errorf("reset token has already been used")
	}

	if time.Now().After(expiresAt) {
		return "", fmt.Errorf("reset token has expired")
	}

	return userID.String(), nil
}

// MarkUsed marks a token as used so it cannot be reused
func (r *PasswordResetRepository) MarkUsed(ctx context.Context, token string) error {
	err := r.session.Query(`
		UPDATE password_reset_tokens SET used = true WHERE reset_token = ?
	`, token).WithContext(ctx).Exec()

	if err != nil {
		return fmt.Errorf("failed to mark token as used: %w", err)
	}

	return nil
}
