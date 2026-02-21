package data

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"

	"social-geo-go/internal/geocoding"
)

// LocationAddress contains full address details
type LocationAddress struct {
	Village      string `json:"village,omitempty"`
	CityDistrict string `json:"city_district,omitempty"`
	City         string `json:"city,omitempty"`
	State        string `json:"state,omitempty"`
	Region       string `json:"region,omitempty"`
	Postcode     string `json:"postcode,omitempty"`
	Country      string `json:"country,omitempty"`
	CountryCode  string `json:"country_code,omitempty"`
}

// LocationName represents cached geocoded location data
type LocationName struct {
	GeohashPrefix string          `json:"geohash_prefix"`
	DisplayName   string          `json:"display_name"`
	Name          string          `json:"name,omitempty"`
	Address       LocationAddress `json:"address"`
	Latitude      float64         `json:"latitude"`
	Longitude     float64         `json:"longitude"`
	CreatedAt     time.Time       `json:"created_at"`
}

// LocationRepository handles location name caching
type LocationRepository struct {
	session  *gocql.Session
	geocoder *geocoding.NominatimClient
}

// NewLocationRepository creates a new LocationRepository
func NewLocationRepository(session *gocql.Session, geocoder *geocoding.NominatimClient) *LocationRepository {
	return &LocationRepository{
		session:  session,
		geocoder: geocoder,
	}
}

// GetByGeohash retrieves a cached location by geohash prefix
func (r *LocationRepository) GetByGeohash(ctx context.Context, geohashPrefix string) (*LocationName, error) {
	var loc LocationName
	var addr LocationAddress

	err := r.session.Query(`
		SELECT geohash_prefix, display_name, name, village, city_district, city, state, region, postcode, country, country_code, latitude, longitude, created_at
		FROM location_names
		WHERE geohash_prefix = ?
	`, geohashPrefix).WithContext(ctx).Scan(
		&loc.GeohashPrefix, &loc.DisplayName, &loc.Name,
		&addr.Village, &addr.CityDistrict, &addr.City, &addr.State, &addr.Region, &addr.Postcode, &addr.Country, &addr.CountryCode,
		&loc.Latitude, &loc.Longitude, &loc.CreatedAt,
	)

	if err != nil {
		if err == gocql.ErrNotFound {
			return nil, nil // Not found, not an error
		}
		return nil, fmt.Errorf("failed to get location: %w", err)
	}

	loc.Address = addr
	return &loc, nil
}

// Save stores a location name in the cache
func (r *LocationRepository) Save(ctx context.Context, loc *LocationName) error {
	return r.session.Query(`
		INSERT INTO location_names (geohash_prefix, display_name, name, village, city_district, city, state, region, postcode, country, country_code, latitude, longitude, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, loc.GeohashPrefix, loc.DisplayName, loc.Name,
		loc.Address.Village, loc.Address.CityDistrict, loc.Address.City, loc.Address.State, loc.Address.Region, loc.Address.Postcode, loc.Address.Country, loc.Address.CountryCode,
		loc.Latitude, loc.Longitude, loc.CreatedAt,
	).WithContext(ctx).Exec()
}

// GetOrFetch retrieves from cache or fetches from Nominatim
func (r *LocationRepository) GetOrFetch(ctx context.Context, geohashPrefix string, lat, lng float64) (*LocationName, error) {
	// Try cache first
	cached, err := r.GetByGeohash(ctx, geohashPrefix)
	if err != nil {
		return nil, err
	}
	if cached != nil {
		return cached, nil
	}

	// Fetch from Nominatim
	info, err := r.geocoder.ReverseGeocode(ctx, lat, lng)
	if err != nil {
		return nil, fmt.Errorf("geocoding failed: %w", err)
	}

	// Create and cache the location
	loc := &LocationName{
		GeohashPrefix: geohashPrefix,
		DisplayName:   info.DisplayName,
		Name:          info.Name,
		Address: LocationAddress{
			Village:      info.Address.Village,
			CityDistrict: info.Address.CityDistrict,
			City:         info.Address.City,
			State:        info.Address.State,
			Region:       info.Address.Region,
			Postcode:     info.Address.Postcode,
			Country:      info.Address.Country,
			CountryCode:  info.Address.CountryCode,
		},
		Latitude:  lat,
		Longitude: lng,
		CreatedAt: time.Now(),
	}

	// Save to cache (ignore error, caching is best-effort)
	_ = r.Save(ctx, loc)

	return loc, nil
}

// GetLocationsByGeohashes batch retrieves locations for multiple geohashes
func (r *LocationRepository) GetLocationsByGeohashes(ctx context.Context, geohashes []string, latLngMap map[string][2]float64) (map[string]*LocationName, error) {
	result := make(map[string]*LocationName)

	for _, gh := range geohashes {
		loc, err := r.GetByGeohash(ctx, gh)
		if err != nil {
			continue
		}
		if loc != nil {
			result[gh] = loc
			continue
		}

		// Not in cache, fetch from Nominatim if we have coordinates
		if coords, ok := latLngMap[gh]; ok {
			loc, _ = r.GetOrFetch(ctx, gh, coords[0], coords[1])
			if loc != nil {
				result[gh] = loc
			}
		}
	}

	return result, nil
}
