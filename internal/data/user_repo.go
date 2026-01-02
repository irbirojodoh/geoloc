package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gocql/gocql"
)

type UserRepository struct {
	session *gocql.Session
}

func NewUserRepository(session *gocql.Session) *UserRepository {
	return &UserRepository{session: session}
}

// CreateUser inserts a new user into the database
func (r *UserRepository) CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error) {
	userID := gocql.TimeUUID()
	now := time.Now()

	err := r.session.Query(`
		INSERT INTO users (id, username, email, full_name, bio, phone_number, profile_picture_url, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, req.Username, req.Email, req.FullName, req.Bio, req.PhoneNumber, req.ProfilePictureURL, req.PasswordHash, now, now).
		WithContext(ctx).Exec()

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &User{
		ID:                userID.String(),
		Username:          req.Username,
		Email:             req.Email,
		FullName:          req.FullName,
		Bio:               req.Bio,
		PhoneNumber:       req.PhoneNumber,
		ProfilePictureURL: req.ProfilePictureURL,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

// GetUserByID retrieves a user by their ID
func (r *UserRepository) GetUserByID(ctx context.Context, id string) (*User, error) {
	userID, err := gocql.ParseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	var user User
	err = r.session.Query(`
		SELECT id, username, email, full_name, bio, phone_number, profile_picture_url, password_hash, created_at, updated_at
		FROM users
		WHERE id = ?
	`, userID).WithContext(ctx).Scan(
		&userID, &user.Username, &user.Email, &user.FullName,
		&user.Bio, &user.PhoneNumber, &user.ProfilePictureURL, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)

	if err != nil {
		if err == gocql.ErrNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	user.ID = userID.String()
	return &user, nil
}

// GetUserByUsername retrieves a user by their username
func (r *UserRepository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	var userID gocql.UUID

	err := r.session.Query(`
		SELECT id, username, email, full_name, bio, phone_number, profile_picture_url, password_hash, created_at, updated_at
		FROM users
		WHERE username = ?
		ALLOW FILTERING
	`, username).WithContext(ctx).Scan(
		&userID, &user.Username, &user.Email, &user.FullName,
		&user.Bio, &user.PhoneNumber, &user.ProfilePictureURL, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)

	if err != nil {
		if err == gocql.ErrNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	user.ID = userID.String()
	return &user, nil
}

// GetUserByEmail retrieves a user by their email
func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	var userID gocql.UUID

	err := r.session.Query(`
		SELECT id, username, email, full_name, bio, phone_number, profile_picture_url, password_hash, created_at, updated_at
		FROM users
		WHERE email = ?
		ALLOW FILTERING
	`, email).WithContext(ctx).Scan(
		&userID, &user.Username, &user.Email, &user.FullName,
		&user.Bio, &user.PhoneNumber, &user.ProfilePictureURL, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)

	if err != nil {
		if err == gocql.ErrNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	user.ID = userID.String()
	return &user, nil
}

// UpdateUser updates user profile fields
func (r *UserRepository) UpdateUser(ctx context.Context, id string, fullName, bio, profilePictureURL string) (*User, error) {
	userID, err := gocql.ParseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	now := time.Now()
	err = r.session.Query(`
		UPDATE users
		SET full_name = ?, bio = ?, profile_picture_url = ?, updated_at = ?
		WHERE id = ?
	`, fullName, bio, profilePictureURL, now, userID).WithContext(ctx).Exec()

	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return r.GetUserByID(ctx, id)
}

// UserExists checks if a user with given ID exists
func (r *UserRepository) UserExists(ctx context.Context, id string) (bool, error) {
	userID, err := gocql.ParseUUID(id)
	if err != nil {
		return false, nil
	}

	var count int
	err = r.session.Query(`
		SELECT COUNT(*) FROM users WHERE id = ?
	`, userID).WithContext(ctx).Scan(&count)

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// UpdateLastSeen updates the user's last online timestamp and IP address
func (r *UserRepository) UpdateLastSeen(ctx context.Context, id, ipAddress string) error {
	userID, err := gocql.ParseUUID(id)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	now := time.Now()
	return r.session.Query(`
		UPDATE users
		SET last_online = ?, last_ip_address = ?
		WHERE id = ?
	`, now, ipAddress, userID).WithContext(ctx).Exec()
}

// SearchUsers searches for users by username
func (r *UserRepository) SearchUsers(ctx context.Context, query string, limit int) ([]User, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	// Cassandra doesn't support LIKE, so we use ALLOW FILTERING with exact match
	// In production, use a search index like Elasticsearch
	iter := r.session.Query(`
		SELECT id, username, email, full_name, bio, profile_picture_url, created_at, updated_at
		FROM users
		LIMIT ?
		ALLOW FILTERING
	`, limit*5).WithContext(ctx).Iter()

	var users []User
	var user User
	var id gocql.UUID

	for iter.Scan(&id, &user.Username, &user.Email, &user.FullName, &user.Bio, &user.ProfilePictureURL, &user.CreatedAt, &user.UpdatedAt) {
		// Client-side filtering for username contains
		if strings.Contains(strings.ToLower(user.Username), strings.ToLower(query)) ||
			strings.Contains(strings.ToLower(user.FullName), strings.ToLower(query)) {
			user.ID = id.String()
			users = append(users, user)
			if len(users) >= limit {
				break
			}
		}
	}

	iter.Close()
	return users, nil
}
