package data

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

// CreateUser inserts a new user into the database
func (r *UserRepository) CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error) {
	query := `
		INSERT INTO users (username, email, full_name, bio)
		VALUES ($1, $2, $3, $4)
		RETURNING id, username, email, full_name, bio, created_at, updated_at
	`

	user := &User{}
	err := r.db.QueryRow(ctx, query, req.Username, req.Email, req.FullName, req.Bio).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.FullName,
		&user.Bio,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// GetUserByID retrieves a user by their ID
func (r *UserRepository) GetUserByID(ctx context.Context, id string) (*User, error) {
	query := `
		SELECT id, username, email, full_name, bio, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	user := &User{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.FullName,
		&user.Bio,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// GetUserByUsername retrieves a user by their username
func (r *UserRepository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	query := `
		SELECT id, username, email, full_name, bio, created_at, updated_at
		FROM users
		WHERE username = $1
	`

	user := &User{}
	err := r.db.QueryRow(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.FullName,
		&user.Bio,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}
