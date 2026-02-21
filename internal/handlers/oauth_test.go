package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"social-geo-go/internal/data"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// MockProvider implements goth.Provider for testing
type MockProvider struct {
	ProviderName string
	UserToReturn goth.User
}

func (m *MockProvider) Name() string        { return m.ProviderName }
func (m *MockProvider) SetName(name string) { m.ProviderName = name }
func (m *MockProvider) BeginAuth(state string) (goth.Session, error) {
	return &MockSession{AuthURL: "http://mock-auth-url", AccessToken: "mock-token"}, nil
}
func (m *MockProvider) FetchUser(session goth.Session) (goth.User, error) {
	return m.UserToReturn, nil
}
func (m *MockProvider) UnmarshalSession(data string) (goth.Session, error) {
	return &MockSession{}, nil
}
func (m *MockProvider) Debug(debug bool) {}
func (m *MockProvider) RefreshToken(refreshToken string) (*oauth2.Token, error) {
	return &oauth2.Token{}, nil
}
func (m *MockProvider) RefreshTokenAvailable() bool { return false }

// MockSession implements goth.Session
type MockSession struct {
	AuthURL     string
	AccessToken string
}

func (s *MockSession) GetAuthURL() (string, error) { return s.AuthURL, nil }
func (s *MockSession) Marshal() string             { return `{"token":"` + s.AccessToken + `"}` }
func (s *MockSession) Authorize(provider goth.Provider, params goth.Params) (string, error) {
	return s.AccessToken, nil
}

func TestOAuthFlow_Integration(t *testing.T) {
	// 1. Setup Dependencies
	// Initialize the session store for Goth (Crucial!)
	key := "test-session-secret"
	os.Setenv("SESSION_SECRET", key)
	os.Setenv("JWT_SECRET", "test-jwt-secret")

	// We need to re-init store for tests to ensure clean state
	gothic.Store = sessions.NewCookieStore([]byte(key))

	// Setup Cassandra Repo
	userRepo := data.NewUserRepository(testSession)

	// 2. Register Mock Providers
	googleMock := &MockProvider{
		ProviderName: "google",
		UserToReturn: goth.User{
			Email:     "test_google@example.com",
			Name:      "Google Test User",
			AvatarURL: "http://google.com/avatar.jpg",
			UserID:    "google-123",
		},
	}

	appleMock := &MockProvider{
		ProviderName: "apple",
		UserToReturn: goth.User{
			Email:     "test_apple@example.com",
			Name:      "Apple Test User",
			AvatarURL: "", // Apple often has no avatar
			UserID:    "apple-456",
		},
	}

	goth.UseProviders(googleMock, appleMock)

	// Configure Gothic Session Store for tests
	store := sessions.NewCookieStore([]byte("test-session-secret"))
	store.MaxAge(300)
	store.Options.Path = "/"
	store.Options.HttpOnly = true
	store.Options.Secure = false // Important for httptest
	gothic.Store = store

	// 3. Setup Router
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Register the exact routes used in main.go
	r.GET("/auth/:provider/login", LoginOAuth())
	r.GET("/auth/:provider/callback", CompleteOAuth(userRepo))
	r.POST("/auth/:provider/callback", CompleteOAuth(userRepo)) // For Apple

	// ==========================================
	// Test Case 1: Google Flow (GET Callback)
	// ==========================================
	t.Run("Google Authentication Flow", func(t *testing.T) {
		// A. Start Login (GET /login)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/auth/google/login", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusTemporaryRedirect, w.Code)

		// Capture the session cookie! This is vital.
		// Gothic sets a cookie with the state verifier. We must pass this back.
		cookies := w.Result().Cookies()
		require.NotEmpty(t, cookies, "Gothic should set a session cookie")
		sessionCookie := cookies[0]

		// B. Callback (GET /callback)
		// We verify the state and complete the login
		wCallback := httptest.NewRecorder()
		reqCallback, _ := http.NewRequest("GET", "/auth/google/callback?state=state&code=mock_code", nil)
		// Add the cookie to the request
		reqCallback.AddCookie(sessionCookie)

		r.ServeHTTP(wCallback, reqCallback)

		// Assertions
		assert.Equal(t, http.StatusOK, wCallback.Code)

		// Verify Response Body contains Token
		body := wCallback.Body.String()
		assert.Contains(t, body, "access_token")
		assert.Contains(t, body, "test_google@example.com")

		// Verify Database Persistence
		user, err := userRepo.GetUserByEmail(req.Context(), "test_google@example.com")
		require.NoError(t, err)
		assert.Equal(t, "Google Test User", user.FullName)
	})

	// ==========================================
	// Test Case 2: Apple Flow (POST Callback)
	// ==========================================
	t.Run("Apple Authentication Flow", func(t *testing.T) {
		// A. Start Login
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/auth/apple/login", nil)
		r.ServeHTTP(w, req)

		cookies := w.Result().Cookies()
		require.NotEmpty(t, cookies)
		sessionCookie := cookies[0]

		// B. Callback (POST request for Apple)
		wCallback := httptest.NewRecorder()
		// Apple sends data as Form values
		form := strings.NewReader("state=state&code=mock_code&user={\"name\":{\"firstName\":\"Apple\",\"lastName\":\"User\"},\"email\":\"test_apple@example.com\"}")
		reqCallback, _ := http.NewRequest("POST", "/auth/apple/callback", form)
		reqCallback.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		reqCallback.AddCookie(sessionCookie)

		r.ServeHTTP(wCallback, reqCallback)

		// Assertions
		assert.Equal(t, http.StatusOK, wCallback.Code)

		// Verify DB
		user, err := userRepo.GetUserByEmail(req.Context(), "test_apple@example.com")
		require.NoError(t, err)
		assert.Equal(t, "Apple Test User", user.FullName)
	})
}
