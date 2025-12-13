package data

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostRepository struct {
	db *pgxpool.Pool
}

func NewPostRepository(db *pgxpool.Pool) *PostRepository {
	return &PostRepository{db: db}
}

// CreatePost inserts a new post with geospatial data
func (r *PostRepository) CreatePost(ctx context.Context, req *CreatePostRequest) (*Post, error) {
	query := `
		INSERT INTO posts (user_id, content, location)
		VALUES ($1, $2, ST_SetSRID(ST_MakePoint($3, $4), 4326))
		RETURNING 
			id, 
			user_id, 
			content, 
			ST_X(location::geometry) as longitude,
			ST_Y(location::geometry) as latitude,
			created_at
	`

	post := &Post{}
	err := r.db.QueryRow(ctx, query, req.UserID, req.Content, req.Longitude, req.Latitude).Scan(
		&post.ID,
		&post.UserID,
		&post.Content,
		&post.Longitude,
		&post.Latitude,
		&post.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create post: %w", err)
	}

	return post, nil
}

// GetNearbyPosts retrieves posts sorted by proximity to a given location
func (r *PostRepository) GetNearbyPosts(ctx context.Context, latitude, longitude, radiusKM float64, limit int) ([]Post, error) {
	// Default values
	if radiusKM <= 0 {
		radiusKM = 10 // Default 10km radius
	}
	if limit <= 0 || limit > 100 {
		limit = 50 // Default 50 posts, max 100
	}

	// Query uses ST_DWithin for spatial filtering and ST_Distance for ordering
	// ST_DWithin uses the spatial index (GiST) for efficient filtering
	query := `
		SELECT 
			id,
			user_id,
			content,
			ST_X(location::geometry) as longitude,
			ST_Y(location::geometry) as latitude,
			created_at,
			ST_Distance(
				location::geography,
				ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography
			) as distance_meters
		FROM posts
		WHERE ST_DWithin(
			location::geography,
			ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography,
			$3
		)
		ORDER BY distance_meters ASC
		LIMIT $4
	`

	radiusMeters := radiusKM * 1000

	rows, err := r.db.Query(ctx, query, longitude, latitude, radiusMeters, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query nearby posts: %w", err)
	}
	defer rows.Close()

	posts := []Post{}
	for rows.Next() {
		var post Post
		var distanceMeters float64
		
		err := rows.Scan(
			&post.ID,
			&post.UserID,
			&post.Content,
			&post.Longitude,
			&post.Latitude,
			&post.CreatedAt,
			&distanceMeters,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan post: %w", err)
		}

		posts = append(posts, post)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating posts: %w", err)
	}

	return posts, nil
}
