package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
)

// RegisterRequest represents the request body for user registration
type RegisterRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=50"`
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=6"`
	FullName    string `json:"full_name" binding:"required"`
	PhoneNumber string `json:"phone_number"`
}

// LoginRequest represents the request body for user login
type LoginRequest struct {
	Identifier string `json:"identifier" binding:"required"` // email or username
	Password   string `json:"password" binding:"required"`
}

// RefreshRequest represents the request body for token refresh
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Register handles POST /auth/register
func Register(userRepo *data.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RegisterRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request body",
			})
			return
		}

		// Check if username already exists
		existing, _ := userRepo.GetUserByUsername(c.Request.Context(), req.Username)
		if existing != nil {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Username already exists",
			})
			return
		}

		// Check if email already exists
		existing, _ = userRepo.GetUserByEmail(c.Request.Context(), req.Email)
		if existing != nil {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Email already exists",
			})
			return
		}

		// Hash password
		passwordHash := auth.HashPassword(req.Password)

		// Create user
		createReq := &data.CreateUserRequest{
			Username:     req.Username,
			Email:        req.Email,
			FullName:     req.FullName,
			PhoneNumber:  req.PhoneNumber,
			PasswordHash: passwordHash,
		}

		user, err := userRepo.CreateUser(c.Request.Context(), createReq)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to create user",
			})
			return
		}

		// Generate tokens
		tokens, err := auth.GenerateTokenPair(user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to generate tokens",
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message":       "User registered successfully",
			"user":          user,
			"access_token":  tokens.AccessToken,
			"refresh_token": tokens.RefreshToken,
			"expires_in":    tokens.ExpiresIn,
		})
	}
}

// Login handles POST /auth/login
func Login(userRepo *data.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request body",
			})
			return
		}

		// Try to find user by email or username
		var user *data.User
		var err error

		// Check if identifier looks like an email
		if strings.Contains(req.Identifier, "@") {
			user, err = userRepo.GetUserByEmail(c.Request.Context(), req.Identifier)
		} else {
			user, err = userRepo.GetUserByUsername(c.Request.Context(), req.Identifier)
		}

		if err != nil || user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid credentials",
			})
			return
		}

		// Verify password
		if !auth.VerifyPassword(req.Password, user.PasswordHash) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid credentials",
			})
			return
		}

		// Generate tokens
		tokens, err := auth.GenerateTokenPair(user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to generate tokens",
			})
			return
		}

		// Update last seen (non-blocking, ignore errors)
		go userRepo.UpdateLastSeen(c.Request.Context(), user.ID, c.ClientIP()) //nolint:errcheck

		c.JSON(http.StatusOK, gin.H{
			"message":       "Login successful",
			"access_token":  tokens.AccessToken,
			"refresh_token": tokens.RefreshToken,
			"expires_in":    tokens.ExpiresIn,
			"user": gin.H{
				"id":       user.ID,
				"username": user.Username,
				"email":    user.Email,
			},
		})
	}
}

// Refresh handles POST /auth/refresh
func Refresh(c *gin.Context) {
	var req RefreshRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body",
		})
		return
	}

	// Validate refresh token and generate new access token
	tokens, err := auth.RefreshAccessToken(req.RefreshToken)
	if err != nil {
		status := http.StatusUnauthorized
		message := "Invalid refresh token"

		if err == auth.ErrExpiredToken {
			message = "Refresh token has expired. Please login again."
		}

		c.JSON(status, gin.H{
			"error": message,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"expires_in":    tokens.ExpiresIn,
	})
}
