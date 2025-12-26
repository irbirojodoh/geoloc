package data

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"
)

type LocationFollowRepository struct {
	session *gocql.Session
}

func NewLocationFollowRepository(session *gocql.Session) *LocationFollowRepository {
	return &LocationFollowRepository{session: session}
}

// FollowLocation subscribes a user to a geographic area
func (r *LocationFollowRepository) FollowLocation(ctx context.Context, userID string, req *FollowLocationRequest) (*LocationFollow, error) {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	geohashPrefix := GetGeohashPrefix(req.Latitude, req.Longitude)
	now := time.Now()

	err = r.session.Query(`
		INSERT INTO location_follows (user_id, geohash_prefix, name, latitude, longitude, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, uid, geohashPrefix, req.Name, req.Latitude, req.Longitude, now).WithContext(ctx).Exec()
	if err != nil {
		return nil, fmt.Errorf("failed to follow location: %w", err)
	}

	return &LocationFollow{
		UserID:        userID,
		GeohashPrefix: geohashPrefix,
		Name:          req.Name,
		Latitude:      req.Latitude,
		Longitude:     req.Longitude,
		CreatedAt:     now,
	}, nil
}

// UnfollowLocation unsubscribes a user from a geographic area
func (r *LocationFollowRepository) UnfollowLocation(ctx context.Context, userID, geohashPrefix string) error {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	return r.session.Query(`
		DELETE FROM location_follows WHERE user_id = ? AND geohash_prefix = ?
	`, uid, geohashPrefix).WithContext(ctx).Exec()
}

// GetFollowedLocations returns all locations a user is following
func (r *LocationFollowRepository) GetFollowedLocations(ctx context.Context, userID string) ([]LocationFollow, error) {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	iter := r.session.Query(`
		SELECT geohash_prefix, name, latitude, longitude, created_at
		FROM location_follows
		WHERE user_id = ?
	`, uid).WithContext(ctx).Iter()

	var locations []LocationFollow
	var loc LocationFollow

	for iter.Scan(&loc.GeohashPrefix, &loc.Name, &loc.Latitude, &loc.Longitude, &loc.CreatedAt) {
		loc.UserID = userID
		locations = append(locations, loc)
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	return locations, nil
}

// GetUsersFollowingLocation returns user IDs following a geohash
func (r *LocationFollowRepository) GetUsersFollowingLocation(ctx context.Context, geohashPrefix string) ([]string, error) {
	iter := r.session.Query(`
		SELECT user_id FROM location_follows WHERE geohash_prefix = ? ALLOW FILTERING
	`, geohashPrefix).WithContext(ctx).Iter()

	var userIDs []string
	var userID gocql.UUID

	for iter.Scan(&userID) {
		userIDs = append(userIDs, userID.String())
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	return userIDs, nil
}
