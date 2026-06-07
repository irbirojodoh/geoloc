package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"context"
	"github.com/gin-gonic/gin"
	"github.com/gocql/gocql"
	"time"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
	"social-geo-go/internal/notifications"
	"social-geo-go/internal/notifications/kafka"
	"social-geo-go/internal/search"
	"social-geo-go/internal/storage"
)

// EnrichPosts adds author, location, and like fields to posts (same shape as GET /api/v1/feed items).
func EnrichPosts(
	ctx context.Context,
	posts []data.Post,
	userRepo *data.UserRepository,
	locRepo *data.LocationRepository,
	likeRepo *data.LikeRepository,
	commentRepo *data.CommentRepository,
	currentUserID string,
	store storage.MediaStore,
) {
	if len(posts) == 0 {
		return
	}

	userIDs := make([]string, 0, len(posts))
	seenUsers := make(map[string]bool)
	geohashes := make([]string, 0, len(posts))
	seenGeohashes := make(map[string]bool)
	latLngMap := make(map[string][2]float64)
	postIDs := make([]string, 0, len(posts))

	for _, p := range posts {
		postIDs = append(postIDs, p.ID)
		if !seenUsers[p.UserID] {
			userIDs = append(userIDs, p.UserID)
			seenUsers[p.UserID] = true
		}
		geohashPrefix := data.GetGeohashPrefix(p.Latitude, p.Longitude)
		if !seenGeohashes[geohashPrefix] {
			geohashes = append(geohashes, geohashPrefix)
			seenGeohashes[geohashPrefix] = true
			latLngMap[geohashPrefix] = [2]float64{p.Latitude, p.Longitude}
		}
	}

	if userRepo != nil {
		userInfoMap, _ := userRepo.GetUsersByIDs(ctx, userIDs)
		for i := range posts {
			if info, ok := userInfoMap[posts[i].UserID]; ok {
				posts[i].Username = info.Username
				posts[i].ProfilePictureURL = storage.ResolveMediaURL(store, info.ProfilePictureURL)
			}
		}
	}

	if locRepo != nil {
		locInfoMap, _ := locRepo.GetLocationsByGeohashes(ctx, geohashes, latLngMap)
		for i := range posts {
			geohashPrefix := data.GetGeohashPrefix(posts[i].Latitude, posts[i].Longitude)
			if loc, ok := locInfoMap[geohashPrefix]; ok {
				posts[i].LocationName = loc.Name
				posts[i].Address = &loc.Address
			}
		}
	}

	if likeRepo != nil {
		likeInfoMap, _ := likeRepo.GetLikesForPosts(ctx, postIDs, currentUserID)
		for i := range posts {
			if info, ok := likeInfoMap[posts[i].ID]; ok {
				posts[i].LikeCount = info.LikeCount
				posts[i].IsLiked = info.IsLiked
			}
		}
	}

	if commentRepo != nil {
		commentCounts, _ := commentRepo.GetCommentCountsForPosts(ctx, postIDs)
		for i := range posts {
			posts[i].CommentCount = commentCounts[posts[i].ID]
		}
	}

	ResolvePostsMediaURLs(store, posts)
}

// CreatePost handles POST /api/v1/posts
func CreatePost(postRepo *data.PostRepository, userRepo *data.UserRepository, notifDispatcher *notifications.NotificationDispatcher, postIndexer search.PostIndexer, store storage.MediaStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req data.CreatePostRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request body",
			})
			return
		}

		// Validate coordinates
		if req.Latitude < -90 || req.Latitude > 90 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Latitude must be between -90 and 90",
			})
			return
		}

		if req.Longitude < -180 || req.Longitude > 180 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Longitude must be between -180 and 180",
			})
			return
		}

		// Validate media (max 4 total)
		if !req.ValidateMedia() {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Maximum 4 media items allowed",
			})
			return
		}

		// Enforce that the post author is the authenticated user — never trust user_id from the request body
		req.UserID = auth.GetUserID(c)
		if req.UserID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			return
		}

		mediaURLs, err := preparePostMedia(store, req.UserID, req.MediaURLs, req.MediaKeys)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		req.MediaURLs = mediaURLs

		// Capture IP address and user agent for tracking
		req.IPAddress = c.ClientIP()
		req.UserAgent = c.Request.UserAgent()

		post, err := postRepo.CreatePost(c.Request.Context(), &req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to create post",
			})
			return
		}

		if notifDispatcher != nil {
			contentTruncated := post.Content
			if len(contentTruncated) > 100 {
				contentTruncated = contentTruncated[:100] + "..."
			}
			go notifDispatcher.DispatchNearbyFanout(context.Background(), &kafka.NearbyFanoutJob{
				EventID:   gocql.TimeUUID().String(),
				PostID:    post.ID,
				AuthorID:  post.UserID,
				Geohash:   post.Geohash,
				Content:   contentTruncated,
				CreatedAt: time.Now().Format(time.RFC3339),
			})
		}

		if postIndexer != nil {
			username := ""
			if user, err := userRepo.GetUserByID(context.Background(), post.UserID); err == nil && user != nil {
				username = user.Username
			}
			event := &search.PostCreatedEvent{
				PostID:    post.ID,
				UserID:    post.UserID,
				Username:  username,
				Content:   post.Content,
				Hashtags:  search.ExtractHashtags(post.Content),
				Lat:       post.Latitude,
				Lon:       post.Longitude,
				Geohash:   post.Geohash,
				CreatedAt: post.CreatedAt,
				LikeCount: 0,
			}
			go func() {
				if err := postIndexer.PublishPostCreated(context.Background(), event); err != nil {
					slog.Warn("failed to publish post created event for search indexing",
						"post_id", post.ID,
						"error", err,
					)
				}
			}()
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Post created successfully",
			"post":    resolvePostForResponse(store, post),
		})
	}
}

func resolvePostForResponse(store storage.MediaStore, post *data.Post) *data.Post {
	if post == nil {
		return nil
	}
	out := *post
	ResolvePostMediaURLs(store, &out)
	return &out
}

func preparePostMedia(store storage.MediaStore, userID string, urls, keys []string) ([]string, error) {
	out := make([]string, 0, len(urls)+len(keys))
	out = append(out, urls...)
	for _, key := range keys {
		if err := validateOwnedMediaKey(key, userID, "posts"); err != nil {
			return nil, err
		}
		out = append(out, storage.StoredMediaValue(store, key))
	}
	return out, nil
}

// GetFeed handles GET /api/v1/feed
func GetFeed(repo *data.PostRepository, userRepo *data.UserRepository, locRepo *data.LocationRepository, likeRepo *data.LikeRepository, commentRepo *data.CommentRepository, modRepo *data.ModerationRepository, store storage.MediaStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req data.GetFeedRequest

		if err := c.ShouldBindQuery(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid query parameters",
			})
			return
		}

		// Validate coordinates
		if req.Latitude < -90 || req.Latitude > 90 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Latitude must be between -90 and 90",
			})
			return
		}

		if req.Longitude < -180 || req.Longitude > 180 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Longitude must be between -180 and 180",
			})
			return
		}

		// Decode cursor for pagination
		cursorTime, err := data.DecodeCursor(req.Cursor)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid cursor",
			})
			return
		}

		// Apply default limit
		limit := data.GetDefaultLimit(req.Limit, 20, 100)

		// Get current user's blocked/muted list for feed filtering
		currentUserID := auth.GetUserID(c)
		var excludedUsers map[string]bool
		if modRepo != nil && currentUserID != "" {
			excludedUsers, _ = modRepo.GetBlockedAndMutedUsers(c.Request.Context(), currentUserID)
		}

		// Fetch extra to account for filtered posts + pagination
		fetchLimit := limit + 1
		if len(excludedUsers) > 0 {
			fetchLimit = limit*2 + 1 // Fetch more if we expect to filter some out
		}

		posts, err := repo.GetNearbyPosts(
			c.Request.Context(),
			req.Latitude,
			req.Longitude,
			req.RadiusKM,
			fetchLimit,
			cursorTime,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to fetch feed",
			})
			return
		}

		// Filter out posts from blocked/muted users
		if len(excludedUsers) > 0 {
			var filtered []data.Post
			for _, p := range posts {
				if !excludedUsers[p.UserID] {
					filtered = append(filtered, p)
				}
			}
			posts = filtered
		}

		// Determine if there are more posts
		hasMore := len(posts) > limit
		if hasMore {
			posts = posts[:limit]
		}

		// Generate next cursor from last post's created_at
		var nextCursor string
		if hasMore && len(posts) > 0 {
			nextCursor = data.EncodeCursor(posts[len(posts)-1].CreatedAt)
		}

		EnrichPosts(c.Request.Context(), posts, userRepo, locRepo, likeRepo, commentRepo, currentUserID, store)

		c.JSON(http.StatusOK, data.PaginatedResponse{
			Data:       posts,
			Count:      len(posts),
			HasMore:    hasMore,
			NextCursor: nextCursor,
		})
	}
}

// GetPost handles GET /api/v1/posts/:id
func GetPost(repo *data.PostRepository, userRepo *data.UserRepository, locRepo *data.LocationRepository, likeRepo *data.LikeRepository, commentRepo *data.CommentRepository, store storage.MediaStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		post, err := repo.GetPostByID(c.Request.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				c.JSON(http.StatusNotFound, gin.H{
					"error": "Post not found",
				})
				return
			}
			slog.Error("Failed to fetch post", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to fetch post",
			})
			return
		}

		// Fetch user details
		user, err := userRepo.GetUserByID(c.Request.Context(), post.UserID)
		if err != nil {
			// If user not found, we still return the post but with limited user info
			// or handle as error depending on business logic. Here we just log and proceed.
			// Ideally every post should have a valid user.
			_ = err // SA9003 empty branch workaround
		}

		// Enrich with location name
		if locRepo != nil {
			geohashPrefix := data.GetGeohashPrefix(post.Latitude, post.Longitude)
			locName, err := locRepo.GetOrFetch(c.Request.Context(), geohashPrefix, post.Latitude, post.Longitude)
			if err == nil && locName != nil {
				post.LocationName = locName.Name
				post.Address = &locName.Address
			}
		}

		// Enrich with like info
		currentUserID := auth.GetUserID(c)
		if likeRepo != nil {
			likeCount, _ := likeRepo.GetLikeCount(c.Request.Context(), data.TargetTypePost, id)
			post.LikeCount = likeCount

			if currentUserID != "" {
				isLiked, _ := likeRepo.HasUserLiked(c.Request.Context(), data.TargetTypePost, id, currentUserID)
				post.IsLiked = isLiked
			}
		}

		if commentRepo != nil {
			commentCount, _ := commentRepo.GetCommentCount(c.Request.Context(), id)
			post.CommentCount = commentCount
		}

		response := gin.H{}

		if user != nil {
			ResolveUserMediaURLs(store, user)
			response["user"] = gin.H{
				"id":                  user.ID,
				"username":            user.Username,
				"full_name":           user.FullName,
				"profile_picture_url": user.ProfilePictureURL,
			}
		}

		resolvedPost := resolvePostForResponse(store, post)
		response["post"] = resolvedPost

		c.JSON(http.StatusOK, response)
	}
}

// GetUserPosts handles GET /api/v1/users/:id/posts
func GetUserPosts(repo *data.PostRepository, userRepo *data.UserRepository, locRepo *data.LocationRepository, likeRepo *data.LikeRepository, commentRepo *data.CommentRepository, store storage.MediaStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("id")

		// Get user info first
		user, err := userRepo.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "User not found",
			})
			return
		}

		// Parse query parameters
		cursor := c.Query("cursor")
		limitStr := c.DefaultQuery("limit", "20")
		var limit int
		fmt.Sscanf(limitStr, "%d", &limit) //nolint:errcheck

		// Decode cursor for pagination
		cursorTime, err := data.DecodeCursor(cursor)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid cursor",
			})
			return
		}

		// Apply default limit
		limit = data.GetDefaultLimit(limit, 20, 100)

		// Fetch one extra to determine if there are more posts
		posts, err := repo.GetPostsByUser(c.Request.Context(), userID, limit+1, cursorTime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to fetch user posts",
			})
			return
		}

		// Determine if there are more posts
		hasMore := len(posts) > limit
		if hasMore {
			posts = posts[:limit]
		}

		// Generate next cursor from last post's created_at
		var nextCursor string
		if hasMore && len(posts) > 0 {
			nextCursor = data.EncodeCursor(posts[len(posts)-1].CreatedAt)
		}

		// Get current user ID for like status
		currentUserID := auth.GetUserID(c)

		// Enrich posts with location and like info
		if len(posts) > 0 {
			geohashes := make([]string, 0, len(posts))
			latLngMap := make(map[string][2]float64)
			postIDs := make([]string, 0, len(posts))

			for _, p := range posts {
				postIDs = append(postIDs, p.ID)
				geohashPrefix := data.GetGeohashPrefix(p.Latitude, p.Longitude)
				geohashes = append(geohashes, geohashPrefix)
				latLngMap[geohashPrefix] = [2]float64{p.Latitude, p.Longitude}
			}

			// Enrich with location info
			if locRepo != nil {
				locInfoMap, _ := locRepo.GetLocationsByGeohashes(c.Request.Context(), geohashes, latLngMap)
				for i := range posts {
					geohashPrefix := data.GetGeohashPrefix(posts[i].Latitude, posts[i].Longitude)
					if loc, ok := locInfoMap[geohashPrefix]; ok {
						posts[i].LocationName = loc.Name
						posts[i].Address = &loc.Address
					}
				}
			}

			// Enrich with like info
			if likeRepo != nil {
				likeInfoMap, _ := likeRepo.GetLikesForPosts(c.Request.Context(), postIDs, currentUserID)
				for i := range posts {
					if info, ok := likeInfoMap[posts[i].ID]; ok {
						posts[i].LikeCount = info.LikeCount
						posts[i].IsLiked = info.IsLiked
					}
				}
			}

			if commentRepo != nil {
				commentCounts, _ := commentRepo.GetCommentCountsForPosts(c.Request.Context(), postIDs)
				for i := range posts {
					posts[i].CommentCount = commentCounts[posts[i].ID]
				}
			}
		}

		ResolvePostsMediaURLs(store, posts)
		ResolveUserMediaURLs(store, user)

		c.JSON(http.StatusOK, gin.H{
			"user": gin.H{
				"id":                  user.ID,
				"username":            user.Username,
				"full_name":           user.FullName,
				"profile_picture_url": user.ProfilePictureURL,
			},
			"count":       len(posts),
			"data":        posts,
			"has_more":    hasMore,
			"next_cursor": nextCursor,
		})
	}
}

// DeletePost handles DELETE /api/v1/posts/:id
func DeletePost(postRepo *data.PostRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		postID := c.Param("id")
		userID := auth.GetUserID(c)

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		err := postRepo.DeletePost(c.Request.Context(), postID, userID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				c.JSON(http.StatusNotFound, gin.H{"error": "Post not found"})
				return
			}
			if strings.Contains(err.Error(), "forbidden") {
				c.JSON(http.StatusForbidden, gin.H{"error": "You can only delete your own posts"})
				return
			}
			slog.Error("Failed to delete post", "error", err, "post_id", postID, "user_id", userID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete post"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Post deleted successfully"})
	}
}
