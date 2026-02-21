package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"

	"github.com/gin-gonic/gin"
	"github.com/markbates/goth/gothic"
)

type contextKey string

const providerKey contextKey = "provider"

// LoginOAuth redirects the user to the provider (Google/Apple)
// /auth/:provider/login
func LoginOAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		provider := c.Param("provider")

		// Inject provider into request context for Gothic to find
		ctx := context.WithValue(c.Request.Context(), providerKey, provider)
		c.Request = c.Request.WithContext(ctx)

		// Start the OAuth dance
		gothic.BeginAuthHandler(c.Writer, c.Request)
	}
}

// CompleteOAuth handles the callback from the provider
// /auth/:provider/callback
func CompleteOAuth(userRepo *data.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		provider := c.Param("provider")

		// Inject provider into request context
		ctx := context.WithValue(c.Request.Context(), providerKey, provider)
		c.Request = c.Request.WithContext(ctx)

		// 1. Complete the auth exchange
		oauthUser, err := gothic.CompleteUserAuth(c.Writer, c.Request)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Authentication failed",
				"details": err.Error(),
			})
			return
		}

		// 2. Find or Create User in DB
		// Goth normalizes data so oauthUser.Email / oauthUser.Name works for both Google & Apple
		user, _, err := userRepo.GetOrCreateOAuthUser(
			c.Request.Context(),
			oauthUser.Email,
			oauthUser.Name,
			oauthUser.AvatarURL,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error", "details": err.Error()})
			return
		}

		// 3. Generate JWT for your App
		tokens, err := auth.GenerateTokenPair(user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Token generation failed"})
			return
		}

		// 4. Redirect to Custom Scheme (for Mobile App)
		frontendURL := os.Getenv("FRONTEND_LOGIN_SUCESS_URL")
		if frontendURL != "" {
			redirectURL := fmt.Sprintf("%s?access_token=%s&refresh_token=%s",
				frontendURL, tokens.AccessToken, tokens.RefreshToken)
			c.Redirect(http.StatusTemporaryRedirect, redirectURL)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"user":   user,
			"tokens": tokens,
		})
	}
}
