package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// Set a dummy secret for testing
	os.Setenv("JWT_SECRET", "test-secret-key-123")
	code := m.Run()
	os.Exit(code)
}

// 1. Test Password Hashing
func TestPasswordHashing(t *testing.T) {
	password := "my-secret-password"

	hash := HashPassword(password)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, password, hash)

	// Verify correct password
	assert.True(t, VerifyPassword(password, hash))

	// Verify wrong password
	assert.False(t, VerifyPassword("wrong-password", hash))
}

// 2. Test TokenType JSON Parsing (The fix you implemented)
func TestTokenTypeJSON(t *testing.T) {
	// Create a struct that uses the TokenType to simulate a request/response
	type TestStruct struct {
		Type TokenType `json:"type"`
	}

	t.Run("Marshal", func(t *testing.T) {
		ts := TestStruct{Type: TokenTypeAccess}
		data, err := json.Marshal(ts)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type": "access"}`, string(data))
	})

	t.Run("Unmarshal Valid Access", func(t *testing.T) {
		jsonStr := `{"type": "access"}`
		var ts TestStruct
		err := json.Unmarshal([]byte(jsonStr), &ts)
		require.NoError(t, err)
		assert.Equal(t, TokenTypeAccess, ts.Type)
	})

	t.Run("Unmarshal Valid Refresh", func(t *testing.T) {
		jsonStr := `{"type": "refresh"}`
		var ts TestStruct
		err := json.Unmarshal([]byte(jsonStr), &ts)
		require.NoError(t, err)
		assert.Equal(t, TokenTypeRefresh, ts.Type)
	})

	t.Run("Unmarshal Invalid", func(t *testing.T) {
		jsonStr := `{"type": "unknown"}`
		var ts TestStruct
		err := json.Unmarshal([]byte(jsonStr), &ts)
		assert.Error(t, err)
		assert.Equal(t, ErrInvalidTokenType, err)
	})
}

// 3. Test Token Generation and Validation
func TestTokenLifecycle(t *testing.T) {
	userID := "user-123-uuid"

	// Generate Pair
	pair, err := GenerateTokenPair(userID)
	require.NoError(t, err)
	assert.NotEmpty(t, pair.AccessToken)
	assert.NotEmpty(t, pair.RefreshToken)

	// Validate Access Token
	accessClaims, err := ValidateAccessToken(pair.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, userID, accessClaims.UserID)
	assert.Equal(t, TokenTypeAccess, accessClaims.Type)

	// Validate Refresh Token
	refreshClaims, err := ValidateRefreshToken(pair.RefreshToken)
	require.NoError(t, err)
	assert.Equal(t, userID, refreshClaims.UserID)
	assert.Equal(t, TokenTypeRefresh, refreshClaims.Type)
}

// 4. Test Token Cross-Validation (Wrong Types)
func TestTokenTypeValidation(t *testing.T) {
	userID := "user-123"
	pair, err := GenerateTokenPair(userID)
	require.NoError(t, err)

	// Try to use Access Token as Refresh Token
	_, err = ValidateRefreshToken(pair.AccessToken)
	assert.ErrorIs(t, err, ErrWrongType)

	// Try to use Refresh Token as Access Token
	_, err = ValidateAccessToken(pair.RefreshToken)
	assert.ErrorIs(t, err, ErrWrongType)
}

// 5. Test Expired Tokens
func TestExpiredToken(t *testing.T) {
	// Manually create an expired token
	claims := &Claims{
		UserID: "user-123",
		Type:   TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)), // Expired 1 hour ago
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(os.Getenv("JWT_SECRET")))

	_, err := ValidateAccessToken(tokenString)
	assert.ErrorIs(t, err, ErrExpiredToken)
}

// 6. Test Middleware
func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup Router
	r := gin.New()
	r.Use(AuthRequired())
	r.GET("/protected", func(c *gin.Context) {
		userID := GetUserID(c)
		c.JSON(200, gin.H{"user_id": userID})
	})

	userID := "test-user-id"
	pair, _ := GenerateTokenPair(userID)

	t.Run("Success", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.JSONEq(t, `{"user_id": "test-user-id"}`, w.Body.String())
	})

	t.Run("Missing Header", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/protected", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, 401, w.Code)
	})

	t.Run("Invalid Format", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/protected", nil)
		req.Header.Set("Authorization", pair.AccessToken) // Missing "Bearer "
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, 401, w.Code)
	})

	t.Run("Invalid Token", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/protected", nil)
		req.Header.Set("Authorization", "Bearer invalid-junk-token")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, 401, w.Code)
	})
}
