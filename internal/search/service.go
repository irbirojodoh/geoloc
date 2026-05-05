package search

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// PostResult is the result from an ES post search, containing indexed fields only.
type PostResult struct {
	PostID    string    `json:"post_id"`
	UserID    string    `json:"user_id"`
	Content   string    `json:"content"`
	Hashtags  []string  `json:"hashtags"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	Geohash   string    `json:"geohash"`
	CreatedAt time.Time `json:"created_at"`
	LikeCount int       `json:"like_count"`
	Score     float64   `json:"_score,omitempty"`
}

// UserResult is the result from an ES user search, containing indexed fields only.
type UserResult struct {
	UserID        string  `json:"user_id"`
	Username      string  `json:"username"`
	DisplayName   string  `json:"display_name"`
	FollowerCount int     `json:"follower_count"`
	IsVerified    bool    `json:"is_verified"`
	AvatarURL     string  `json:"avatar_url,omitempty"`
	Score         float64 `json:"_score,omitempty"`
}

// Service defines the search operations available to API handlers.
type Service interface {
	SearchPosts(ctx context.Context, q string, lat, lon float64, radiusKm float64) ([]PostResult, error)
	SearchUsers(ctx context.Context, q string) ([]UserResult, error)
	AutocompleteUsernames(ctx context.Context, prefix string) ([]string, error)
	AutocompleteHashtags(ctx context.Context, prefix string) ([]string, error)
}

type searchService struct {
	es         *ESClient
	redis      *redis.Client
	postsIndex string
	usersIndex string
	maxResults int
}

// NewService creates a new search Service.
func NewService(es *ESClient, rdb *redis.Client) Service {
	postsIndex := os.Getenv("ELASTICSEARCH_INDEX_POSTS")
	if postsIndex == "" {
		postsIndex = "posts"
	}
	usersIndex := os.Getenv("ELASTICSEARCH_INDEX_USERS")
	if usersIndex == "" {
		usersIndex = "users"
	}
	maxResults := 20
	if v := os.Getenv("SEARCH_MAX_RESULTS"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &maxResults); err != nil || n != 1 {
			maxResults = 20
		}
	}

	return &searchService{
		es:         es,
		redis:      rdb,
		postsIndex: postsIndex,
		usersIndex: usersIndex,
		maxResults: maxResults,
	}
}

// SearchPosts searches for posts by content/hashtags with optional geo filtering.
func (s *searchService) SearchPosts(ctx context.Context, q string, lat, lon float64, radiusKm float64) ([]PostResult, error) {
	start := time.Now()
	defer func() {
		slog.Debug("search posts latency", "query", q, "lat", lat, "lon", lon, "elapsed_ms", time.Since(start).Milliseconds())
	}()

	// Build the ES query
	mustClause := map[string]interface{}{
		"multi_match": map[string]interface{}{
			"query":     q,
			"fields":    []string{"content^2", "hashtags"},
			"fuzziness": "AUTO",
		},
	}

	boolQuery := map[string]interface{}{
		"must": mustClause,
	}

	// Add geo filter if lat/lon are non-zero
	if lat != 0 || lon != 0 {
		if radiusKm <= 0 {
			radiusKm = 5 // default radius
		}
		boolQuery["filter"] = map[string]interface{}{
			"geo_distance": map[string]interface{}{
				"distance": fmt.Sprintf("%.1fkm", radiusKm),
				"location": map[string]float64{
					"lat": lat,
					"lon": lon,
				},
			},
		}
	}

	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": boolQuery,
		},
		"sort": []interface{}{
			map[string]interface{}{"_score": "desc"},
			map[string]interface{}{"created_at": map[string]string{"order": "desc"}},
		},
		"size": s.maxResults,
		"_source": []string{
			"post_id", "user_id", "content", "hashtags",
			"location", "geohash", "created_at", "like_count",
		},
	}

	data, err := s.es.Search(ctx, s.postsIndex, query)
	if err != nil {
		return nil, fmt.Errorf("search posts: %w", err)
	}

	esResp, err := ParseESResponse(data)
	if err != nil {
		return nil, fmt.Errorf("search posts parse: %w", err)
	}

	results := make([]PostResult, 0, len(esResp.Hits.Hits))
	for _, hit := range esResp.Hits.Hits {
		pr := PostResult{
			PostID:  safeString(hit.Source, "post_id"),
			UserID:  safeString(hit.Source, "user_id"),
			Content: safeString(hit.Source, "content"),
			Geohash: safeString(hit.Source, "geohash"),
			Score:   hit.Score,
		}

		// Parse hashtags
		if ht, ok := hit.Source["hashtags"]; ok {
			switch v := ht.(type) {
			case []interface{}:
				for _, tag := range v {
					if t, ok := tag.(string); ok {
						pr.Hashtags = append(pr.Hashtags, t)
					}
				}
			}
		}

		// Parse location
		if loc, ok := hit.Source["location"]; ok {
			switch l := loc.(type) {
			case map[string]interface{}:
				pr.Lat, _ = l["lat"].(float64)
				pr.Lon, _ = l["lon"].(float64)
			}
		}

		// Parse created_at
		if ca, ok := hit.Source["created_at"]; ok {
			if dateStr, ok := ca.(string); ok {
				pr.CreatedAt, _ = time.Parse(time.RFC3339, dateStr)
			}
		}

		// Parse like_count
		if lc, ok := hit.Source["like_count"]; ok {
			switch v := lc.(type) {
			case float64:
				pr.LikeCount = int(v)
			}
		}

		results = append(results, pr)
	}

	slog.Info("search posts complete", "query", q, "results", len(results))
	return results, nil
}

// SearchUsers searches for users by username or display name.
func (s *searchService) SearchUsers(ctx context.Context, q string) ([]UserResult, error) {
	start := time.Now()
	defer func() {
		slog.Debug("search users latency", "query", q, "elapsed_ms", time.Since(start).Milliseconds())
	}()

	query := map[string]interface{}{
		"query": map[string]interface{}{
			"function_score": map[string]interface{}{
				"query": map[string]interface{}{
					"multi_match": map[string]interface{}{
						"query":     q,
						"fields":    []string{"username", "display_name"},
						"fuzziness": "AUTO",
					},
				},
				"functions": []interface{}{
					map[string]interface{}{
						"filter": map[string]interface{}{
							"term": map[string]bool{"is_verified": true},
						},
						"weight": 1.5,
					},
				},
				"boost_mode": "multiply",
			},
		},
		"sort": []interface{}{
			map[string]interface{}{"_score": "desc"},
			map[string]interface{}{"follower_count": map[string]string{"order": "desc"}},
		},
		"size": s.maxResults,
		"_source": []string{
			"user_id", "username", "display_name",
			"follower_count", "is_verified", "avatar_url",
		},
	}

	data, err := s.es.Search(ctx, s.usersIndex, query)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}

	esResp, err := ParseESResponse(data)
	if err != nil {
		return nil, fmt.Errorf("search users parse: %w", err)
	}

	results := make([]UserResult, 0, len(esResp.Hits.Hits))
	for _, hit := range esResp.Hits.Hits {
		ur := UserResult{
			UserID:    safeString(hit.Source, "user_id"),
			Username:  safeString(hit.Source, "username"),
			AvatarURL: safeString(hit.Source, "avatar_url"),
			Score:     hit.Score,
		}

		if dn, ok := hit.Source["display_name"]; ok {
			ur.DisplayName, _ = dn.(string)
		}
		if fc, ok := hit.Source["follower_count"]; ok {
			if v, ok := fc.(float64); ok {
				ur.FollowerCount = int(v)
			}
		}
		if iv, ok := hit.Source["is_verified"]; ok {
			ur.IsVerified, _ = iv.(bool)
		}

		results = append(results, ur)
	}

	slog.Info("search users complete", "query", q, "results", len(results))
	return results, nil
}

// AutocompleteUsernames returns username suggestions from Redis sorted set.
func (s *searchService) AutocompleteUsernames(ctx context.Context, prefix string) ([]string, error) {
	if s.redis == nil {
		return nil, fmt.Errorf("redis not available")
	}

	start := time.Now()
	defer func() {
		slog.Debug("autocomplete usernames latency", "prefix", prefix, "elapsed_ms", time.Since(start).Milliseconds())
	}()

	// ZRANGEBYLEX users:autocomplete "[prefix" "[prefix\xff" LIMIT 0 10
	min := fmt.Sprintf("[%s", prefix)
	max := fmt.Sprintf("[%s\xff", prefix)

	results, err := s.redis.ZRangeByLex(ctx, "users:autocomplete", &redis.ZRangeBy{
		Min:    min,
		Max:    max,
		Offset: 0,
		Count:  10,
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("redis autocomplete: %w", err)
	}

	// Strip the \xff suffix from results
	for i, r := range results {
		if len(r) > 0 && r[len(r)-1] == '\xff' {
			results[i] = r[:len(r)-1]
		}
	}

	slog.Info("autocomplete usernames complete", "prefix", prefix, "results", len(results))
	return results, nil
}

// AutocompleteHashtags returns hashtag suggestions from ES aggregations.
func (s *searchService) AutocompleteHashtags(ctx context.Context, prefix string) ([]string, error) {
	start := time.Now()
	defer func() {
		slog.Debug("autocomplete hashtags latency", "prefix", prefix, "elapsed_ms", time.Since(start).Milliseconds())
	}()

	// Use a prefix query on hashtags with a terms aggregation
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"prefix": map[string]interface{}{
				"hashtags": prefix,
			},
		},
		"aggs": map[string]interface{}{
			"hashtag_suggestions": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "hashtags",
					"size":  10,
					"order": map[string]string{"_count": "desc"},
				},
			},
		},
		"size": 0,
	}

	data, err := s.es.Search(ctx, s.postsIndex, query)
	if err != nil {
		return nil, fmt.Errorf("autocomplete hashtags: %w", err)
	}

	esResp, err := ParseESResponse(data)
	if err != nil {
		return nil, fmt.Errorf("autocomplete hashtags parse: %w", err)
	}

	// Extract aggregation results
	var hashtags []string
	if agg, ok := esResp.Aggregations["hashtag_suggestions"]; ok {
		aggBytes, err := json.Marshal(agg)
		if err != nil {
			return nil, fmt.Errorf("marshal aggregation: %w", err)
		}
		var termsAgg struct {
			Buckets []struct {
				Key      string `json:"key"`
				DocCount int    `json:"doc_count"`
			} `json:"buckets"`
		}
		if err := json.Unmarshal(aggBytes, &termsAgg); err != nil {
			return nil, fmt.Errorf("unmarshal aggregation buckets: %w", err)
		}
		for _, bucket := range termsAgg.Buckets {
			hashtags = append(hashtags, "#"+bucket.Key)
		}
	}

	slog.Info("autocomplete hashtags complete", "prefix", prefix, "results", len(hashtags))
	return hashtags, nil
}

func safeString(source map[string]interface{}, key string) string {
	if v, ok := source[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
