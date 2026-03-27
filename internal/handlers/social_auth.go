package handlers

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
)

// SocialLoginRequest is the body for mobile-native social sign-in endpoints.
// The mobile app obtains an ID token from the native SDK and sends it here.
type SocialLoginRequest struct {
	// IDToken is the ID token obtained from the Google Sign-In or Sign in with Apple SDK.
	IDToken string `json:"id_token" binding:"required"`
	// FullName is only needed for Apple Sign-In on the user's first login.
	// Apple only returns the user's name in the native callback on first sign-in;
	// subsequent logins do not include name info.
	FullName string `json:"full_name"`
}

// GoogleLogin handles POST /auth/google/token
// Mobile app sends the Google ID token obtained from google_sign_in Flutter package.
//
// Request body: { "id_token": "eyJ..." }
// Response:     { "user": {...}, "access_token": "...", "refresh_token": "...", "is_new_user": true }
func GoogleLogin(userRepo *data.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SocialLoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "id_token is required"})
			return
		}

		clientID := os.Getenv("GOOGLE_CLIENT_ID")
		if clientID == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Google sign-in is not configured"})
			return
		}

		// Verify the ID token server-side with Google
		socialUser, err := auth.VerifyGoogleIDToken(c.Request.Context(), req.IDToken, clientID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired Google token"})
			return
		}

		// Find or create the local user record
		user, isNew, err := userRepo.GetOrCreateOAuthUser(
			c.Request.Context(),
			socialUser.Email,
			socialUser.FullName,
			socialUser.AvatarURL,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process sign-in"})
			return
		}

		// Issue app JWT tokens
		tokens, err := auth.GenerateTokenPair(user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"user":          user,
			"access_token":  tokens.AccessToken,
			"refresh_token": tokens.RefreshToken,
			"expires_in":    tokens.ExpiresIn,
			"is_new_user":   isNew,
		})
	}
}

// AppleLogin handles POST /auth/apple/token
// Mobile app sends the Apple ID token from Sign in with Apple (sign_in_with_apple Flutter package).
//
// Request body: { "id_token": "eyJ...", "full_name": "Jane Doe" }
// Note: full_name should be sent on first sign-in only; Apple won't include it in future logins.
// Response:     { "user": {...}, "access_token": "...", "refresh_token": "...", "is_new_user": true }
func AppleLogin(userRepo *data.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SocialLoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "id_token is required"})
			return
		}

		clientID := os.Getenv("APPLE_CLIENT_ID")
		if clientID == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Apple sign-in is not configured"})
			return
		}

		// Verify the ID token using Apple's JWKS public keys
		socialUser, err := auth.VerifyAppleIDToken(c.Request.Context(), req.IDToken, clientID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired Apple token"})
			return
		}

		// Apple only provides name on the very first sign-in via native SDK callback.
		// The Flutter app must pass it in this request on first login.
		fullName := socialUser.FullName
		if fullName == "" && req.FullName != "" {
			fullName = req.FullName
		}

		// Find or create user
		user, isNew, err := userRepo.GetOrCreateOAuthUser(
			c.Request.Context(),
			socialUser.Email,
			fullName,
			socialUser.AvatarURL,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process sign-in"})
			return
		}

		// Issue app JWT tokens
		tokens, err := auth.GenerateTokenPair(user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"user":          user,
			"access_token":  tokens.AccessToken,
			"refresh_token": tokens.RefreshToken,
			"expires_in":    tokens.ExpiresIn,
			"is_new_user":   isNew,
		})
	}
}
