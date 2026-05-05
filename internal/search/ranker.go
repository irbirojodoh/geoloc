package search

import (
	"math"
	"sort"
	"time"
)

// RankPosts applies a final re-ranking pass to search results using a weighted scoring formula:
//
//	final_score = (0.5 × es_score_normalized)
//	           + (0.3 × recency_score)
//	           + (0.2 × proximity_score)
//
// Results are sorted in descending order by final_score.
func RankPosts(results []PostResult, userLat, userLon float64) []PostResult {
	if len(results) == 0 {
		return results
	}

	// Find max ES score for normalization
	maxScore := 0.0
	for _, r := range results {
		if r.Score > maxScore {
			maxScore = r.Score
		}
	}
	if maxScore == 0 {
		maxScore = 1.0 // avoid division by zero
	}

	now := time.Now()

	type scored struct {
		post  PostResult
		score float64
	}

	scoredResults := make([]scored, len(results))
	for i, r := range results {
		esScoreNorm := r.Score / maxScore

		// Recency score: 1 / (1 + hours_since_posted)
		hoursSincePosted := now.Sub(r.CreatedAt).Hours()
		if hoursSincePosted < 0 {
			hoursSincePosted = 0
		}
		recencyScore := 1.0 / (1.0 + hoursSincePosted)

		// Proximity score: 1 / (1 + distance_km), 0.0 if no geo context
		proximityScore := 0.0
		if userLat != 0 || userLon != 0 {
			distanceKm := haversineDistance(userLat, userLon, r.Lat, r.Lon)
			proximityScore = 1.0 / (1.0 + distanceKm)
		}

		finalScore := 0.5*esScoreNorm + 0.3*recencyScore + 0.2*proximityScore

		scoredResults[i] = scored{post: r, score: finalScore}
	}

	// Sort by final score descending
	sort.Slice(scoredResults, func(i, j int) bool {
		return scoredResults[i].score > scoredResults[j].score
	})

	// Extract sorted results
	ranked := make([]PostResult, len(scoredResults))
	for i, sr := range scoredResults {
		ranked[i] = sr.post
	}

	return ranked
}

// haversineDistance calculates the great-circle distance between two points
// on the Earth (specified in decimal degrees) in kilometers.
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371.0 // km

	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}
