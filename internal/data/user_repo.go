package data

import (
	"context"
	"fmt"
	"log/slog"
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

// UserInfo contains basic user info for enriching posts
type UserInfo struct {
	Username          string
	ProfilePictureURL string
}

func (r *UserRepository) GetOrCreateOAuthUser(ctx context.Context, email, fullName, avatarURL string) (*User, bool, error) {
	if email == "" {
		return nil, false, fmt.Errorf("email is required from provider")
	}

	existingUser, err := r.GetUserByEmail(ctx, email)
	if err == nil && existingUser != nil {
		slog.Info(fmt.Sprintf("[OAUTH] User %s from OAuth already exist in the database", email))
		// User exists - Login successful
		return existingUser, false, nil
	}

	// 3. User does not exist - Prepare for creation
	userID := gocql.TimeUUID()
	now := time.Now()

	// 4. Generate a unique username
	// Strategy: Use the name part of the email + random timestamp to ensure uniqueness
	// e.g., "john.doe" -> "john.doe_173546"
	baseName := strings.Split(email, "@")[0]
	// Sanitize: replace non-alphanumeric chars with underscore
	baseName = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, baseName)

	username := fmt.Sprintf("%s_%d", baseName, now.Unix()%100000)

	// 5. Insert new user into Cassandra
	// Note: Password hash is empty string "" effectively disabling password login for this account
	query := `
		INSERT INTO users (
			id, 
			username, 
			email, 
			full_name, 
			bio, 
			profile_picture_url, 
			password_hash, 
			created_at, 
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	err = r.session.Query(query,
		userID,
		username,
		email,
		fullName,
		"Joined via Social Login", // Default bio
		avatarURL,
		"", // Empty password hash
		now,
		now,
	).WithContext(ctx).Exec()

	if err != nil {
		return nil, false, fmt.Errorf("failed to create oauth user: %w", err)
	}

	// 6. Return the newly created user object
	newUser := &User{
		ID:                userID.String(),
		Username:          username,
		Email:             email,
		FullName:          fullName,
		Bio:               "Joined via Social Login",
		ProfilePictureURL: avatarURL,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	return newUser, true, nil
}

// GetUsersByIDs retrieves usernames and profile pictures for multiple user IDs
func (r *UserRepository) GetUsersByIDs(ctx context.Context, userIDs []string) (map[string]UserInfo, error) {
	result := make(map[string]UserInfo)

	for _, idStr := range userIDs {
		userID, err := gocql.ParseUUID(idStr)
		if err != nil {
			continue
		}

		var username, profilePicURL string
		err = r.session.Query(`
			SELECT username, profile_picture_url FROM users WHERE id = ?
		`, userID).WithContext(ctx).Scan(&username, &profilePicURL)

		if err == nil {
			result[idStr] = UserInfo{
				Username:          username,
				ProfilePictureURL: profilePicURL,
			}
		}
	}

	return result, nil
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
	// Apply default cover image if empty
	if user.CoverImageURL == "" {
		user.CoverImageURL = DefaultCoverImageURL
	}
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
	// Apply default cover image if empty
	if user.CoverImageURL == "" {
		user.CoverImageURL = DefaultCoverImageURL
	}
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
	// Apply default cover image if empty
	if user.CoverImageURL == "" {
		user.CoverImageURL = DefaultCoverImageURL
	}
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

	// Utilize Cassandra SAI indexes for exact matching (case-insensitive)
	iter := r.session.Query(`
		SELECT id, username, email, full_name, bio, profile_picture_url, created_at, updated_at
		FROM users
		WHERE username = ?
	`, query).WithContext(ctx).Iter()

	var users []User
	var user User
	var id gocql.UUID

	for iter.Scan(&id, &user.Username, &user.Email, &user.FullName, &user.Bio, &user.ProfilePictureURL, &user.CreatedAt, &user.UpdatedAt) {
		user.ID = id.String()
		// Apply default cover image if empty
		if user.CoverImageURL == "" {
			user.CoverImageURL = DefaultCoverImageURL
		}
		users = append(users, user)
		if len(users) >= limit {
			break
		}
	}

	iter.Close()
	return users, nil
}
