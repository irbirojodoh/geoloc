package data

import (
	"math"

	"github.com/mmcloughlin/geohash"
)

const (
	// DefaultGeohashPrecision is 5 characters (~5km cell size)
	DefaultGeohashPrecision = 5
	// EarthRadiusKM is the radius of Earth in kilometers
	EarthRadiusKM = 6371.0
)

// EncodeGeohash converts latitude and longitude to a geohash string
func EncodeGeohash(lat, lng float64, precision uint) string {
	return geohash.EncodeWithPrecision(lat, lng, precision)
}

// GetGeohashPrefix returns the prefix of a geohash at specified precision
func GetGeohashPrefix(lat, lng float64) string {
	return EncodeGeohash(lat, lng, DefaultGeohashPrecision)
}

// GetNeighbors returns the center geohash plus all 8 neighboring cells
// This is necessary because a point near the edge of a cell might have
// nearby posts in adjacent cells
func GetNeighbors(lat, lng float64) []string {
	centerHash := GetGeohashPrefix(lat, lng)
	neighbors := geohash.Neighbors(centerHash)

	// Neighbors returns []string with 8 neighbors, prepend center
	result := make([]string, 0, 9)
	result = append(result, centerHash)
	result = append(result, neighbors...)

	return result
}

// HaversineDistance calculates the distance between two points on Earth
// using the Haversine formula. Returns distance in kilometers.
func HaversineDistance(lat1, lng1, lat2, lng2 float64) float64 {
	// Convert to radians
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLng := (lng2 - lng1) * math.Pi / 180

	// Haversine formula
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLng/2)*math.Sin(deltaLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return EarthRadiusKM * c
}

// IsWithinRadius checks if a point is within the specified radius (in km)
func IsWithinRadius(centerLat, centerLng, pointLat, pointLng, radiusKM float64) bool {
	return HaversineDistance(centerLat, centerLng, pointLat, pointLng) <= radiusKM
}
