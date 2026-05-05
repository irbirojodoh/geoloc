package search
package search

import (
	"math"
	"testing"
	"time"
)

func TestRankPosts_EmptyInput(t *testing.T) {
	results := RankPosts(nil, 0, 0)
	if results != nil {
		t.Errorf("expected nil, got %v", results)
	}

	results = RankPosts([]PostResult{}, 0, 0)
	if len(results) != 0 {
		t.Errorf("expected empty, got %d", len(results))
	}
}

func TestRankPosts_SinglePost(t *testing.T) {
	now := time.Now()
	posts := []PostResult{
		{
			PostID:    "1",
			Content:   "test post",
			CreatedAt: now.Add(-1 * time.Hour),
			Score:     5.0,
		},
	}

	results := RankPosts(posts, 0, 0)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].PostID != "1" {
		t.Errorf("expected post ID 1, got %s", results[0].PostID)
	}
}

func TestRankPosts_OrdersByScore(t *testing.T) {
	now := time.Now()
	posts := []PostResult{
		{
			PostID:    "low-score",
			Content:   "old low score",
			CreatedAt: now.Add(-10 * time.Hour),
			Score:     2.0,
			Lat:       40.0,
			Lon:       -74.0,
		},
		{
			PostID:    "high-score",
			Content:   "recent high score",
			CreatedAt: now.Add(-1 * time.Hour),
			Score:     10.0,
			Lat:       40.0,
			Lon:       -74.0,
		},
	}

	results := RankPosts(posts, 40.0, -74.0)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].PostID != "high-score" {
		t.Errorf("expected high-score first, got %s", results[0].PostID)
	}
}

func TestRankPosts_RecencyBoost(t *testing.T) {
	now := time.Now()
	// Both have same ES score
	posts := []PostResult{
		{
			PostID:    "old-post",
			Content:   "old post",
			CreatedAt: now.Add(-48 * time.Hour), // 2 days ago
			Score:     10.0,
		},
		{
			PostID:    "new-post",
			Content:   "new post",
			CreatedAt: now.Add(-1 * time.Hour), // 1 hour ago
			Score:     10.0,
		},
	}

	results := RankPosts(posts, 0, 0)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].PostID != "new-post" {
		t.Errorf("expected new-post first (recency boost), got %s", results[0].PostID)
	}
}

func TestRankPosts_ProximityBoost(t *testing.T) {
	now := time.Now()
	// Both have same ES score and same created_at
	posts := []PostResult{
		{
			PostID:    "far-post",
			Content:   "far away",
			CreatedAt: now.Add(-1 * time.Hour),
			Score:     10.0,
			Lat:       34.0,
			Lon:       -118.0, // Los Angeles - far from user
		},
		{
			PostID:    "near-post",
			Content:   "nearby",
			CreatedAt: now.Add(-1 * time.Hour),
			Score:     10.0,
			Lat:       40.0,
			Lon:       -74.0, // New York - close to user
		},
	}

	// User is in New York
	results := RankPosts(posts, 40.71, -74.01)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].PostID != "near-post" {
		t.Errorf("expected near-post first (proximity boost), got %s", results[0].PostID)
	}
}

func TestRankPosts_NoGeoContext(t *testing.T) {
	now := time.Now()
	posts := []PostResult{
		{
			PostID:    "post-1",
			Content:   "first",
			CreatedAt: now.Add(-5 * time.Hour),
			Score:     8.0,
			Lat:       40.0,
			Lon:       -74.0,
		},
		{
			PostID:    "post-2",
			Content:   "second",
			CreatedAt: now.Add(-2 * time.Hour),
			Score:     5.0,
			Lat:       34.0,
			Lon:       -118.0,
		},
	}

	// No user geo context — proximity score should be 0 for both
	results := RankPosts(posts, 0, 0)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// post-1 has higher ES score so should rank first
	if results[0].PostID != "post-1" {
		t.Errorf("expected post-1 first (higher ES score), got %s", results[0].PostID)
	}
}

func TestHaversineDistance(t *testing.T) {
	// New York to New York = 0
	d := haversineDistance(40.7128, -74.0060, 40.7128, -74.0060)
	if math.Abs(d) > 0.001 {
		t.Errorf("expected 0 distance, got %.2f", d)
	}

	// New York to Los Angeles ~3944 km
	d = haversineDistance(40.7128, -74.0060, 34.0522, -118.2437)
	if d < 3900 || d > 4000 {
		t.Errorf("expected ~3944 km, got %.2f", d)
	}

	// London to Paris ~344 km
	d = haversineDistance(51.5074, -0.1278, 48.8566, 2.3522)
	if d < 330 || d > 360 {
		t.Errorf("expected ~344 km, got %.2f", d)
	}
}

func TestRankPosts_Stability(t *testing.T) {
	now := time.Now()
	posts := []PostResult{
		{PostID: "a", Content: "a", CreatedAt: now.Add(-1 * time.Hour), Score: 5.0},
		{PostID: "b", Content: "b", CreatedAt: now.Add(-2 * time.Hour), Score: 5.0},
		{PostID: "c", Content: "c", CreatedAt: now.Add(-3 * time.Hour), Score: 5.0},
	}

	// All have same ES score, different recency — should be ordered by recency
	results := RankPosts(posts, 0, 0)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	expected := []string{"a", "b", "c"}
	for i, id := range expected {
		if results[i].PostID != id {
			t.Errorf("position %d: expected %s, got %s", i, id, results[i].PostID)
		}
	}
}

func TestRankPosts_MaxScoreNormalization(t *testing.T) {
	now := time.Now()
	posts := []PostResult{
		{
			PostID:    "top",
			Content:   "top",
			CreatedAt: now.Add(-10 * time.Hour),
			Score:     100.0,
		},
		{
			PostID:    "middle",
			Content:   "middle",
			CreatedAt: now.Add(-10 * time.Hour),
			Score:     50.0,
		},
		{
			PostID:    "bottom",
			Content:   "bottom",
			CreatedAt: now.Add(-10 * time.Hour),
			Score:     10.0,
		},
	}

	// Same recency, same geo context — should be ordered by normalized ES score
	results := RankPosts(posts, 0, 0)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	expected := []string{"top", "middle", "bottom"}
	for i, id := range expected {
		if results[i].PostID != id {
			t.Errorf("position %d: expected %s, got %s", i, id, results[i].PostID)
		}
	}
}
