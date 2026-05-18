package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gocql/gocql"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
	"social-geo-go/internal/search"
)

// SearchUsers handles GET /api/v1/search/users (legacy Cassandra-backed search)
func SearchUsers(userRepo *data.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := strings.TrimSpace(c.Query("q"))
		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Search query is required"})
			return
		}

		if len(query) < 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Search query must be at least 2 characters"})
			return
		}

		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
				limit = parsed
			}
		}

		users, err := userRepo.SearchUsers(c.Request.Context(), query, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Search failed"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"query":   query,
			"results": users,
			"count":   len(users),
		})
	}
}

// SearchPosts handles GET /api/v1/search/posts (legacy Cassandra-backed search)
func SearchPosts(postRepo *data.PostRepository, userRepo *data.UserRepository, likeRepo *data.LikeRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := strings.TrimSpace(c.Query("q"))
		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Search query is required"})
			return
		}

		if len(query) < 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Search query must be at least 2 characters"})
			return
		}

		limit := 20
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
				limit = parsed
			}
		}

		posts, err := postRepo.SearchPosts(c.Request.Context(), query, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Search failed"})
			return
		}

		// Enrich posts with author info
		if len(posts) > 0 {
			postIDs := make([]string, 0, len(posts))
			userIDs := make([]string, 0, len(posts))
			for _, p := range posts {
				userIDs = append(userIDs, p.UserID)
				postIDs = append(postIDs, p.ID)
			}
			userInfoMap, _ := userRepo.GetUsersByIDs(c.Request.Context(), userIDs)

			currentUserID, _ := c.Get("user_id")
			var uid string
			if id, ok := currentUserID.(string); ok {
				uid = id
			}

			likeInfo, _ := likeRepo.GetLikesForPosts(c.Request.Context(), postIDs, uid)

			for i := range posts {
				if info, ok := userInfoMap[posts[i].UserID]; ok {
					posts[i].Username = info.Username
					posts[i].ProfilePictureURL = info.ProfilePictureURL
				}
				if info, ok := likeInfo[posts[i].ID]; ok {
					posts[i].IsLiked = info.IsLiked
					posts[i].LikeCount = info.LikeCount
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"query":   query,
			"results": posts,
			"count":   len(posts),
		})
	}
}

// NewSearchHandler holds dependencies for the new ES-backed search routes.
type NewSearchHandler struct {
	svc      search.Service
	session  *gocql.Session
	userRepo *data.UserRepository
	locRepo  *data.LocationRepository
	likeRepo *data.LikeRepository
}

// NewNewSearchHandler creates a new NewSearchHandler.
func NewNewSearchHandler(
	svc search.Service,
	session *gocql.Session,
	userRepo *data.UserRepository,
	locRepo *data.LocationRepository,
	likeRepo *data.LikeRepository,
) *NewSearchHandler {
	return &NewSearchHandler{
		svc:      svc,
		session:  session,
		userRepo: userRepo,
		locRepo:  locRepo,
		likeRepo: likeRepo,
	}
}

func (h *NewSearchHandler) hydrateAndEnrichPosts(
	ctx context.Context,
	esPosts []search.PostResult,
	currentUserID string,
	viewerLat, viewerLon float64,
) []data.Post {
	postIDs := make([]string, 0, len(esPosts))
	distanceByPostID := make(map[string]float64, len(esPosts))
	for _, p := range esPosts {
		postIDs = append(postIDs, p.PostID)
		if viewerLat != 0 || viewerLon != 0 {
			distanceByPostID[p.PostID] = search.HaversineDistance(viewerLat, viewerLon, p.Lat, p.Lon)
		}
	}

	hydratedPosts, _ := search.HydratePosts(ctx, postIDs, h.session)
	EnrichPosts(ctx, hydratedPosts, h.userRepo, h.locRepo, h.likeRepo, currentUserID)

	if len(distanceByPostID) > 0 {
		for i := range hydratedPosts {
			if d, ok := distanceByPostID[hydratedPosts[i].ID]; ok {
				hydratedPosts[i].Distance = d
			}
		}
	}

	return hydratedPosts
}

// SearchHandler handles GET /v1/search
func (h *NewSearchHandler) SearchHandler(c *gin.Context) {
	start := time.Now()
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query parameter 'q' is required"})
		return
	}
	if len(q) < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query must be at least 1 character"})
		return
	}

	searchType := c.DefaultQuery("type", "all")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}

	ctx := c.Request.Context()

	var (
		posts    []search.PostResult
		users    []search.UserResult
		postsErr error
		usersErr error
	)
	var wg sync.WaitGroup

	searchPosts := searchType == "all" || searchType == "posts"
	searchUsers := searchType == "all" || searchType == "users"

	if searchPosts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			posts, postsErr = h.svc.SearchPosts(ctx, q, 0, 0, 0)
		}()
	}

	if searchUsers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			users, usersErr = h.svc.SearchUsers(ctx, q)
		}()
	}

	wg.Wait()

	// Return 503 if both queries failed — graceful degradation
	if postsErr != nil && usersErr != nil {
		slog.Error("search: both queries failed", "posts_err", postsErr, "users_err", usersErr)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "search unavailable"})
		return
	}
	if postsErr != nil {
		slog.Warn("search: posts query failed, returning partial results", "error", postsErr)
	}
	if usersErr != nil {
		slog.Warn("search: users query failed, returning partial results", "error", usersErr)
	}

	currentUserID := auth.GetUserID(c)
	var hydratedPosts []data.Post
	if len(posts) > 0 {
		hydratedPosts = h.hydrateAndEnrichPosts(ctx, posts, currentUserID, 0, 0)
	}

	// Hydrate users from Cassandra
	userIDs := make([]string, 0, len(users))
	for _, u := range users {
		userIDs = append(userIDs, u.UserID)
	}
	hydratedUsers, _ := search.HydrateUsers(ctx, userIDs, h.session)

	total := len(hydratedPosts) + len(hydratedUsers)

	elapsed := time.Since(start)
	slog.Info("search request complete",
		"query", q,
		"type", searchType,
		"posts", len(hydratedPosts),
		"users", len(hydratedUsers),
		"elapsed_ms", elapsed.Milliseconds(),
	)

	c.JSON(http.StatusOK, gin.H{
		"posts": hydratedPosts,
		"users": hydratedUsers,
		"total": total,
		"query": q,
	})
}

// SearchNearbyHandler handles GET /v1/search/nearby
func (h *NewSearchHandler) SearchNearbyHandler(c *gin.Context) {
	start := time.Now()
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query parameter 'q' is required"})
		return
	}

	latStr := c.Query("lat")
	lonStr := c.Query("lon")
	if latStr == "" || lonStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Parameters 'lat' and 'lon' are required"})
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid 'lat' value"})
		return
	}
	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid 'lon' value"})
		return
	}

	radiusKm, _ := strconv.ParseFloat(c.DefaultQuery("radius_km", "5"), 64)
	if radiusKm <= 0 {
		radiusKm = 5
	}

	searchType := c.DefaultQuery("type", "all")
	ctx := c.Request.Context()

	var (
		posts    []search.PostResult
		users    []search.UserResult
		postsErr error
		usersErr error
	)
	var wg sync.WaitGroup

	searchPosts := searchType == "all" || searchType == "posts"
	searchUsers := searchType == "all" || searchType == "users"

	if searchPosts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			posts, postsErr = h.svc.SearchPosts(ctx, q, lat, lon, radiusKm)
		}()
	}

	if searchUsers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			users, usersErr = h.svc.SearchUsers(ctx, q)
		}()
	}

	wg.Wait()

	if postsErr != nil && usersErr != nil {
		slog.Error("search nearby: both queries failed", "posts_err", postsErr, "users_err", usersErr)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "search unavailable"})
		return
	}
	if postsErr != nil {
		slog.Warn("search nearby: posts query failed", "error", postsErr)
	}
	if usersErr != nil {
		slog.Warn("search nearby: users query failed", "error", usersErr)
	}

	// Apply re-ranking for posts with geo context
	if len(posts) > 0 {
		posts = search.RankPosts(posts, lat, lon)
	}

	currentUserID := auth.GetUserID(c)
	var hydratedPosts []data.Post
	if len(posts) > 0 {
		hydratedPosts = h.hydrateAndEnrichPosts(ctx, posts, currentUserID, lat, lon)
	}

	// Hydrate users
	userIDs := make([]string, 0, len(users))
	for _, u := range users {
		userIDs = append(userIDs, u.UserID)
	}
	hydratedUsers, _ := search.HydrateUsers(ctx, userIDs, h.session)

	total := len(hydratedPosts) + len(hydratedUsers)

	elapsed := time.Since(start)
	slog.Info("search nearby complete",
		"query", q,
		"lat", lat,
		"lon", lon,
		"radius_km", radiusKm,
		"posts", len(hydratedPosts),
		"users", len(hydratedUsers),
		"elapsed_ms", elapsed.Milliseconds(),
	)

	c.JSON(http.StatusOK, gin.H{
		"posts": hydratedPosts,
		"users": hydratedUsers,
		"total": total,
		"query": q,
	})
}

// AutocompleteHandler handles GET /v1/autocomplete
func (h *NewSearchHandler) AutocompleteHandler(c *gin.Context) {
	start := time.Now()
	q := strings.TrimSpace(c.Query("q"))
	if q == "" || len(q) < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query parameter 'q' is required (min 1 character)"})
		return
	}

	searchType := c.DefaultQuery("type", "all")
	ctx := c.Request.Context()

	var (
		usernames    []string
		hashtags     []string
		usernamesErr error
		hashtagsErr  error
	)
	var wg sync.WaitGroup

	searchUsers := searchType == "all" || searchType == "users"
	searchHashtags := searchType == "all" || searchType == "hashtags"

	if searchUsers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			usernames, usernamesErr = h.svc.AutocompleteUsernames(ctx, q)
		}()
	}

	if searchHashtags {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hashtags, hashtagsErr = h.svc.AutocompleteHashtags(ctx, q)
		}()
	}

	wg.Wait()

	if usernamesErr != nil {
		slog.Warn("autocomplete usernames failed", "error", usernamesErr)
	}
	if hashtagsErr != nil {
		slog.Warn("autocomplete hashtags failed", "error", hashtagsErr)
	}

	elapsed := time.Since(start)
	slog.Info("autocomplete complete",
		"prefix", q,
		"type", searchType,
		"usernames", len(usernames),
		"hashtags", len(hashtags),
		"elapsed_ms", elapsed.Milliseconds(),
	)

	c.JSON(http.StatusOK, gin.H{
		"users":    usernames,
		"hashtags": hashtags,
	})
}
