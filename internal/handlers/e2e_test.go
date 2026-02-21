package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
	"social-geo-go/internal/push"
	"social-geo-go/internal/storage"
)

// ============== TEST HELPERS ==============

// setupE2ERouter creates a gin router with all routes wired up, matching main.go
func setupE2ERouter() *gin.Engine {
	os.Setenv("JWT_SECRET", "test-jwt-secret-for-e2e")

	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Repositories
	userRepo := data.NewUserRepository(testSession)
	postRepo := data.NewPostRepository(testSession)
	commentRepo := data.NewCommentRepository(testSession)
	followRepo := data.NewFollowRepository(testSession)
	likeRepo := data.NewLikeRepository(testSession, nil) // nil redis for tests
	notifRepo := data.NewNotificationRepository(testSession)
	locRepo := data.NewLocationRepository(testSession, nil) // nil geocoder for tests
	locFollowRepo := data.NewLocationFollowRepository(testSession)
	store := storage.NewLocalStorage("/tmp/test-uploads", "http://localhost:8080/uploads")
	pushService := push.NewLogPushService()

	// Public routes
	r.POST("/auth/register", Register(userRepo))
	r.POST("/auth/login", Login(userRepo))
	r.POST("/auth/refresh", Refresh)

	// Protected routes
	api := r.Group("/api/v1")
	api.Use(auth.AuthRequired())
	{
		// Feed
		api.GET("/feed", GetFeed(postRepo, userRepo, locRepo, likeRepo))

		// Profile
		api.GET("/users/me", GetCurrentUser(userRepo))
		api.PUT("/users/me", UpdateProfile(userRepo))

		// Users
		api.GET("/users/:id", GetUser(userRepo))
		api.GET("/users/username/:username", GetUserByUsername(userRepo))
		api.GET("/users/:id/posts", GetUserPosts(postRepo, userRepo, locRepo, likeRepo))

		// Follow
		api.POST("/users/:id/follow", FollowUser(followRepo, notifRepo))
		api.DELETE("/users/:id/follow", UnfollowUser(followRepo))
		api.GET("/users/:id/followers", GetFollowers(followRepo))
		api.GET("/users/:id/following", GetFollowing(followRepo))

		// Posts
		api.POST("/posts", CreatePost(postRepo, userRepo))
		api.GET("/posts/:id", GetPost(postRepo, userRepo, locRepo, likeRepo))

		// data.Post likes
		api.POST("/posts/:id/like", LikePost(likeRepo))
		api.DELETE("/posts/:id/like", UnlikePost(likeRepo))
		api.POST("/posts/:id/toggle-like", TogglePostLike(likeRepo))

		// Comments
		api.POST("/posts/:id/comments", CreateComment(commentRepo))
		api.GET("/posts/:id/comments", GetComments(commentRepo))

		// data.Comment actions
		api.POST("/comments/:id/reply", ReplyToComment(commentRepo))
		api.POST("/comments/:id/like", LikeComment(likeRepo))
		api.DELETE("/comments/:id/like", UnlikeComment(likeRepo))
		api.POST("/comments/:id/toggle-like", ToggleCommentLike(likeRepo))
		api.DELETE("/comments/:id", DeleteComment(commentRepo))

		// Search
		api.GET("/search/users", SearchUsers(userRepo))
		api.GET("/search/posts", SearchPosts(postRepo))

		// Notifications
		api.GET("/notifications", GetNotifications(notifRepo))
		api.PUT("/notifications/:id/read", MarkNotificationAsRead(notifRepo))

		// data.Location follows
		api.POST("/locations/:geohash/follow", FollowLocation(locFollowRepo))
		api.DELETE("/locations/:geohash/follow", UnfollowLocation(locFollowRepo))
		api.GET("/locations/following", GetFollowedLocations(locFollowRepo))

		// Upload
		api.POST("/upload/avatar", UploadAvatar(store))
		api.POST("/upload/post", UploadPostMedia(store))

		// Device
		api.POST("/devices", RegisterDevice(pushService))
		api.DELETE("/devices", UnregisterDevice(pushService))
	}

	return r
}

// registerAndLogin creates a user and returns the access token
func registerAndLogin(t *testing.T, router *gin.Engine, username, email, password string) (string, string) {
	t.Helper()

	// Register
	body, _ := json.Marshal(map[string]string{
		"username":  username,
		"email":     email,
		"password":  password,
		"full_name": "Test " + username,
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "register failed: %s", w.Body.String())

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp) //nolint:errcheck

	token := resp["access_token"].(string)
	user := resp["user"].(map[string]interface{})
	userID := user["id"].(string)

	return token, userID
}

// authedRequest creates an HTTP request with Bearer token
func authedRequest(method, url string, body interface{}, token string) *http.Request {
	var reqBody *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}
	req, _ := http.NewRequest(method, url, reqBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// ============== AUTH TESTS ==============

func TestE2E_Auth_Register(t *testing.T) {
	router := setupE2ERouter()

	t.Run("Success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"username":  "e2e_register_ok",
			"email":     "e2e_register_ok@test.com",
			"password":  "password123",
			"full_name": "Register Test",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp) //nolint:errcheck
		assert.NotEmpty(t, resp["access_token"])
		assert.NotEmpty(t, resp["refresh_token"])
		assert.NotNil(t, resp["user"])
	})

	t.Run("Duplicate Username", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"username":  "e2e_register_ok",
			"email":     "different@test.com",
			"password":  "password123",
			"full_name": "Duplicate",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("Invalid Body", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"username": "ab", // too short (min 3)
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestE2E_Auth_Login(t *testing.T) {
	router := setupE2ERouter()

	// Create user first
	registerAndLogin(t, router, "e2e_login_user", "e2e_login@test.com", "password123")

	t.Run("Login by Username", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"identifier": "e2e_login_user",
			"password":   "password123",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp) //nolint:errcheck
		assert.NotEmpty(t, resp["access_token"])
		assert.Equal(t, "Login successful", resp["message"])
	})

	t.Run("Login by Email", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"identifier": "e2e_login@test.com",
			"password":   "password123",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Wrong Password", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"identifier": "e2e_login_user",
			"password":   "wrongpassword",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Nonexistent data.User", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"identifier": "nobody_exists_here",
			"password":   "password123",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestE2E_Auth_Refresh(t *testing.T) {
	router := setupE2ERouter()

	// Register and get tokens
	body, _ := json.Marshal(map[string]string{
		"username":  "e2e_refresh_user",
		"email":     "e2e_refresh@test.com",
		"password":  "password123",
		"full_name": "Refresh Test",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	var regResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &regResp)
	refreshToken := regResp["refresh_token"].(string)

	t.Run("Valid Refresh", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"refresh_token": refreshToken,
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/auth/refresh", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NotEmpty(t, resp["access_token"])
	})

	t.Run("Invalid Refresh Token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"refresh_token": "invalid.token.here",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/auth/refresh", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ============== USER TESTS ==============

func TestE2E_Users(t *testing.T) {
	router := setupE2ERouter()
	token, userID := registerAndLogin(t, router, "e2e_user_test", "e2e_user@test.com", "password123")

	t.Run("Get Current data.User", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("GET", "/api/v1/users/me", nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		user := resp["user"].(map[string]interface{})
		assert.Equal(t, "e2e_user_test", user["username"])
	})

	t.Run("Get data.User By ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("GET", "/api/v1/users/"+userID, nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Get data.User By Username", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("GET", "/api/v1/users/username/e2e_user_test", nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Update Profile", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("PUT", "/api/v1/users/me", map[string]string{
			"full_name": "Updated Name",
			"bio":       "Updated bio from E2E test",
		}, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		user := resp["user"].(map[string]interface{})
		assert.Equal(t, "Updated Name", user["full_name"])
	})

	t.Run("Unauthorized Without Token", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/users/me", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ============== POST TESTS ==============

func TestE2E_Posts(t *testing.T) {
	router := setupE2ERouter()
	token, userID := registerAndLogin(t, router, "e2e_post_user", "e2e_post@test.com", "password123")

	var postID string

	t.Run("Create data.Post", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("POST", "/api/v1/posts", map[string]interface{}{
			"user_id":   userID,
			"content":   "E2E test post content",
			"latitude":  -6.2088,
			"longitude": 106.8456,
		}, token)
		router.ServeHTTP(w, req)

		t.Logf("Create data.Post response: %s", w.Body.String())
		require.Equal(t, http.StatusCreated, w.Code, "create post failed: %s", w.Body.String())

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		post := resp["post"].(map[string]interface{})
		postID = post["id"].(string)
		assert.NotEmpty(t, postID)
		assert.Equal(t, "E2E test post content", post["content"])
	})

	t.Run("Get data.Post", func(t *testing.T) {
		require.NotEmpty(t, postID)

		w := httptest.NewRecorder()
		req := authedRequest("GET", "/api/v1/posts/"+postID, nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		post := resp["post"].(map[string]interface{})
		assert.Equal(t, "E2E test post content", post["content"])
	})

	t.Run("Get data.User Posts", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("GET", "/api/v1/users/"+userID+"/posts", nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Get Feed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("GET", "/api/v1/feed?latitude=-6.2088&longitude=106.8456", nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Create data.Post Invalid Body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("POST", "/api/v1/posts", map[string]string{
			"content": "", // Missing required fields
		}, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ============== COMMENT TESTS ==============

func TestE2E_Comments(t *testing.T) {
	router := setupE2ERouter()
	token, userID := registerAndLogin(t, router, "e2e_comment_user", "e2e_comment@test.com", "password123")

	// Create a post first
	w := httptest.NewRecorder()
	req := authedRequest("POST", "/api/v1/posts", map[string]interface{}{
		"content":   "data.Post for comments test",
		"user_id":   userID,
		"latitude":  -6.2088,
		"longitude": 106.8456,
	}, token)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var postResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &postResp)
	postID := postResp["post"].(map[string]interface{})["id"].(string)

	var commentID string

	t.Run("Create data.Comment", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("POST", "/api/v1/posts/"+postID+"/comments", map[string]string{
			"content": "E2E test comment",
		}, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		comment := resp["comment"].(map[string]interface{})
		commentID = comment["id"].(string)
		assert.NotEmpty(t, commentID)
	})

	t.Run("Get Comments", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("GET", "/api/v1/posts/"+postID+"/comments", nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Reply to data.Comment", func(t *testing.T) {
		require.NotEmpty(t, commentID)

		w := httptest.NewRecorder()
		req := authedRequest("POST", "/api/v1/comments/"+commentID+"/reply", map[string]interface{}{
			"content": "E2E test reply",
			"post_id": postID,
		}, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("Delete data.Comment", func(t *testing.T) {
		require.NotEmpty(t, commentID)

		w := httptest.NewRecorder()
		req := authedRequest("DELETE", "/api/v1/comments/"+commentID, nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ============== LIKE TESTS ==============

func TestE2E_Likes(t *testing.T) {
	router := setupE2ERouter()
	token, userID := registerAndLogin(t, router, "e2e_like_user", "e2e_like@test.com", "password123")

	// Create a post
	w := httptest.NewRecorder()
	req := authedRequest("POST", "/api/v1/posts", map[string]interface{}{
		"user_id":   userID,
		"content":   "data.Post for likes test",
		"latitude":  -6.2088,
		"longitude": 106.8456,
	}, token)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var postResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &postResp)
	postID := postResp["post"].(map[string]interface{})["id"].(string)

	t.Run("Like data.Post", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("POST", "/api/v1/posts/"+postID+"/like", nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("Unlike data.Post", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("DELETE", "/api/v1/posts/"+postID+"/like", nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Toggle data.Post Like", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("POST", "/api/v1/posts/"+postID+"/toggle-like", nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NotNil(t, resp["is_liked"])
	})

	// Create a comment, then like it
	t.Run("Like and Unlike data.Comment", func(t *testing.T) {
		// Create comment
		w := httptest.NewRecorder()
		req := authedRequest("POST", "/api/v1/posts/"+postID+"/comments", map[string]string{
			"content": "data.Comment to like",
		}, token)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)

		var commentResp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &commentResp)
		commentID := commentResp["comment"].(map[string]interface{})["id"].(string)

		// Like comment
		w = httptest.NewRecorder()
		req = authedRequest("POST", "/api/v1/comments/"+commentID+"/like", nil, token)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)

		// Unlike comment
		w = httptest.NewRecorder()
		req = authedRequest("DELETE", "/api/v1/comments/"+commentID+"/like", nil, token)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// Toggle comment like
		w = httptest.NewRecorder()
		req = authedRequest("POST", "/api/v1/comments/"+commentID+"/toggle-like", nil, token)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ============== FOLLOW TESTS ==============

func TestE2E_Follow(t *testing.T) {
	router := setupE2ERouter()
	token1, _ := registerAndLogin(t, router, "e2e_follow_a", "e2e_follow_a@test.com", "password123")
	_, userID2 := registerAndLogin(t, router, "e2e_follow_b", "e2e_follow_b@test.com", "password123")

	t.Run("Follow data.User", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("POST", "/api/v1/users/"+userID2+"/follow", nil, token1)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Get Followers", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("GET", "/api/v1/users/"+userID2+"/followers", nil, token1)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Get Following", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("GET", "/api/v1/users/"+userID2+"/following", nil, token1)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Unfollow data.User", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("DELETE", "/api/v1/users/"+userID2+"/follow", nil, token1)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Cannot Follow Self", func(t *testing.T) {
		// Get user1's ID
		w := httptest.NewRecorder()
		req := authedRequest("GET", "/api/v1/users/me", nil, token1)
		router.ServeHTTP(w, req)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		userID1 := resp["user"].(map[string]interface{})["id"].(string)

		w = httptest.NewRecorder()
		req = authedRequest("POST", "/api/v1/users/"+userID1+"/follow", nil, token1)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ============== SEARCH TESTS ==============

func TestE2E_Search(t *testing.T) {
	router := setupE2ERouter()
	token, userID := registerAndLogin(t, router, "e2e_searchable", "e2e_search@test.com", "password123")

	t.Run("Search Users", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("GET", "/api/v1/search/users?q=e2e_search", nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["results"] != nil {
			users := resp["results"].([]interface{})
			assert.GreaterOrEqual(t, len(users), 1)
		} else {
			t.Errorf("Expected results, got nil: %s", w.Body.String())
		}
	})

	t.Run("Search Users Empty Query", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("GET", "/api/v1/search/users?q=", nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Search Posts", func(t *testing.T) {
		// Create a post to search
		w := httptest.NewRecorder()
		req := authedRequest("POST", "/api/v1/posts", map[string]interface{}{
			"user_id":   userID,
			"content":   "Searchable E2E post about Go programming",
			"latitude":  -6.2088,
			"longitude": 106.8456,
		}, token)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)

		// Search for it
		w = httptest.NewRecorder()
		req = authedRequest("GET", "/api/v1/search/posts?q=Searchable", nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ============== NOTIFICATION TESTS ==============

func TestE2E_Notifications(t *testing.T) {
	router := setupE2ERouter()
	token, _ := registerAndLogin(t, router, "e2e_notif_user", "e2e_notif@test.com", "password123")

	t.Run("Get Notifications", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("GET", "/api/v1/notifications", nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ============== LOCATION FOLLOW TESTS ==============

func TestE2E_LocationFollow(t *testing.T) {
	router := setupE2ERouter()
	token, _ := registerAndLogin(t, router, "e2e_loc_user", "e2e_loc@test.com", "password123")

	geohash := "qqgu6"

	t.Run("Follow data.Location", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("POST", "/api/v1/locations/"+geohash+"/follow", map[string]interface{}{
			"latitude":  -6.2088,
			"longitude": 106.8456,
			"name":      "Jakarta",
		}, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("Get Followed Locations", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("GET", "/api/v1/locations/following", nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Unfollow data.Location", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := authedRequest("DELETE", "/api/v1/locations/"+geohash+"/follow", nil, token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ============== ERROR DETAIL LEAK PREVENTION ==============

func TestE2E_NoErrorDetailsLeaked(t *testing.T) {
	router := setupE2ERouter()
	token, _ := registerAndLogin(t, router, "e2e_error_user", "e2e_error@test.com", "password123")

	// Test that error responses don't contain "details" key with raw error messages
	endpoints := []struct {
		method string
		url    string
	}{
		{"GET", "/api/v1/users/invalid-uuid"},
		{"GET", "/api/v1/posts/invalid-uuid"},
		{"POST", "/api/v1/users/invalid-uuid/follow"},
	}

	for _, ep := range endpoints {
		t.Run(fmt.Sprintf("%s %s", ep.method, ep.url), func(t *testing.T) {
			w := httptest.NewRecorder()
			req := authedRequest(ep.method, ep.url, nil, token)
			router.ServeHTTP(w, req)

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)

			// Must not contain "details" key (error detail leak)
			_, hasDetails := resp["details"]
			assert.False(t, hasDetails, "Response should not contain 'details' key: %s", w.Body.String())
		})
	}
}
