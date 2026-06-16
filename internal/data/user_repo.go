package data

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gocql/gocql"
)

// ErrAccountDeleted is returned when attempting to access a soft-deleted account
var ErrAccountDeleted = fmt.Errorf("account has been deleted")

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
		SELECT id, username, email, full_name, bio, phone_number, profile_picture_url, cover_image_url, password_hash, is_deleted, created_at, updated_at
		FROM users
		WHERE id = ?
	`, userID).WithContext(ctx).Scan(
		&userID, &user.Username, &user.Email, &user.FullName,
		&user.Bio, &user.PhoneNumber, &user.ProfilePictureURL, &user.CoverImageURL, &user.PasswordHash, &user.IsDeleted, &user.CreatedAt, &user.UpdatedAt,
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
		SELECT id, username, email, full_name, bio, phone_number, profile_picture_url, cover_image_url, password_hash, is_deleted, created_at, updated_at
		FROM users
		WHERE username = ?
		ALLOW FILTERING
	`, username).WithContext(ctx).Scan(
		&userID, &user.Username, &user.Email, &user.FullName,
		&user.Bio, &user.PhoneNumber, &user.ProfilePictureURL, &user.CoverImageURL, &user.PasswordHash, &user.IsDeleted, &user.CreatedAt, &user.UpdatedAt,
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
		SELECT id, username, email, full_name, bio, phone_number, profile_picture_url, cover_image_url, password_hash, is_deleted, created_at, updated_at
		FROM users
		WHERE email = ?
		ALLOW FILTERING
	`, email).WithContext(ctx).Scan(
		&userID, &user.Username, &user.Email, &user.FullName,
		&user.Bio, &user.PhoneNumber, &user.ProfilePictureURL, &user.CoverImageURL, &user.PasswordHash, &user.IsDeleted, &user.CreatedAt, &user.UpdatedAt,
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

// UpdateUser updates user profile fields including avatar and cover image URLs.
func (r *UserRepository) UpdateUser(ctx context.Context, id string, fullName, bio, phoneNumber, profilePictureURL, coverImageURL string) (*User, error) {
	userID, err := gocql.ParseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	now := time.Now()
	err = r.session.Query(`
		UPDATE users
		SET full_name = ?, bio = ?, phone_number = ?, profile_picture_url = ?, cover_image_url = ?, updated_at = ?
		WHERE id = ?
	`, fullName, bio, phoneNumber, profilePictureURL, coverImageURL, now, userID).WithContext(ctx).Exec()

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

// SearchUsers searches for users by username or full_name using SAI prefix matching.
// Runs two parallel queries (username LIKE, full_name LIKE) and deduplicates.
func (r *UserRepository) SearchUsers(ctx context.Context, query string, limit int) ([]User, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	if normalizedQuery == "" {
		return nil, nil
	}

	// Normalize query for LIKE prefix matching
	searchPattern := normalizedQuery + "%"

	// Search by username (prefix match via SAI)
	usernameIter := r.session.Query(`
		SELECT id, username, email, full_name, bio, profile_picture_url, created_at, updated_at
		FROM users
		WHERE username LIKE ?
		LIMIT ?
	`, searchPattern, limit).WithContext(ctx).Iter()

	seen := make(map[string]bool)
	var users []User

	var user User
	var id gocql.UUID
	for usernameIter.Scan(&id, &user.Username, &user.Email, &user.FullName,
		&user.Bio, &user.ProfilePictureURL, &user.CreatedAt, &user.UpdatedAt) {
		idStr := id.String()
		if !seen[idStr] {
			user.ID = idStr
			if user.CoverImageURL == "" {
				user.CoverImageURL = DefaultCoverImageURL
			}
			// Filter out soft-deleted users
			if !user.IsDeleted {
				users = append(users, user)
				seen[idStr] = true
			}
		}
		user = User{}
		if len(users) >= limit {
			break
		}
	}
	if err := usernameIter.Close(); err != nil && len(users) == 0 {
		slog.Warn("username search query failed, falling back to scan", "error", err, "query", query)
	}

	// If we haven't hit the limit, also search by full_name
	if len(users) < limit {
		remaining := limit - len(users)
		nameIter := r.session.Query(`
			SELECT id, username, email, full_name, bio, profile_picture_url, created_at, updated_at
			FROM users
			WHERE full_name LIKE ?
			LIMIT ?
		`, searchPattern, remaining).WithContext(ctx).Iter()

		for nameIter.Scan(&id, &user.Username, &user.Email, &user.FullName,
			&user.Bio, &user.ProfilePictureURL, &user.CreatedAt, &user.UpdatedAt) {
			idStr := id.String()
			if !seen[idStr] {
				user.ID = idStr
				if user.CoverImageURL == "" {
					user.CoverImageURL = DefaultCoverImageURL
				}
				if !user.IsDeleted {
					users = append(users, user)
					seen[idStr] = true
				}
			}
			user = User{}
			if len(users) >= limit {
				break
			}
		}
		if err := nameIter.Close(); err != nil && len(users) == 0 {
			slog.Warn("full_name search query failed, falling back to scan", "error", err, "query", query)
		}
	}

	if len(users) > 0 {
		return users, nil
	}

	return r.searchUsersByScan(ctx, normalizedQuery, limit)
}

func (r *UserRepository) searchUsersByScan(ctx context.Context, query string, limit int) ([]User, error) {
	scanLimit := limit * 20
	if scanLimit < 100 {
		scanLimit = 100
	}
	if scanLimit > 1000 {
		scanLimit = 1000
	}

	iter := r.session.Query(`
		SELECT id, username, email, full_name, bio, profile_picture_url, created_at, updated_at
		FROM users
		LIMIT ?
	`, scanLimit).WithContext(ctx).Iter()

	var users []User
	var user User
	var id gocql.UUID
	for iter.Scan(&id, &user.Username, &user.Email, &user.FullName,
		&user.Bio, &user.ProfilePictureURL, &user.CreatedAt, &user.UpdatedAt) {
		if strings.Contains(strings.ToLower(user.Username), query) || strings.Contains(strings.ToLower(user.FullName), query) {
			user.ID = id.String()
			if user.CoverImageURL == "" {
				user.CoverImageURL = DefaultCoverImageURL
			}
			if !user.IsDeleted {
				users = append(users, user)
			}
		}
		user = User{}
		if len(users) >= limit {
			break
		}
	}
	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("search users fallback scan failed: %w", err)
	}

	return users, nil
}

// UpdatePassword updates a user's password hash
func (r *UserRepository) UpdatePassword(ctx context.Context, userID, newPasswordHash string) error {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	now := time.Now()
	return r.session.Query(`
		UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?
	`, newPasswordHash, now, uid).WithContext(ctx).Exec()
}

// SoftDeleteUser anonymizes PII and marks the account as deleted
func (r *UserRepository) SoftDeleteUser(ctx context.Context, userID string) error {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	now := time.Now()
	prefix := userID[:8] // Use first 8 chars of UUID for anonymized username

	err = r.session.Query(`
		UPDATE users SET
			username = ?,
			email = ?,
			full_name = ?,
			bio = ?,
			phone_number = ?,
			profile_picture_url = ?,
			password_hash = ?,
			is_deleted = ?,
			deleted_at = ?,
			updated_at = ?
		WHERE id = ?
	`,
		"deleted_"+prefix,                  // anonymized username
		"deleted_"+userID+"@deleted.local", // anonymized email
		"Deleted User",                     // anonymized name
		"",                                 // clear bio
		"",                                 // clear phone
		"",                                 // clear avatar
		"",                                 // disable password login
		true,                               // mark as deleted
		now,                                // deletion timestamp
		now,                                // updated_at
		uid,
	).WithContext(ctx).Exec()

	if err != nil {
		return fmt.Errorf("failed to soft-delete user: %w", err)
	}

	// Delete key backup entry to satisfy GDPR requirement
	err = r.session.Query(`
		DELETE FROM user_dm_identity_backups WHERE user_id = ?
	`, uid).WithContext(ctx).Exec()
	if err != nil {
		return fmt.Errorf("failed to delete user key backup: %w", err)
	}

	slog.Info("[ACCOUNT] User soft-deleted, PII anonymized and key backups cleared", "user_id", userID)
	return nil
}
