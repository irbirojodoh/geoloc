package auth

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrExpiredToken     = errors.New("token has expired")
	ErrWrongType        = errors.New("wrong token type")
	ErrInvalidTokenType = errors.New("invalid token type")
)

type TokenType int

const (
	TokenTypeAccess TokenType = iota
	TokenTypeRefresh
)

var TokenTypeName = map[TokenType]string{
	TokenTypeAccess:  "access",
	TokenTypeRefresh: "refresh",
}

func (t TokenType) String() string {
	return TokenTypeName[t]
}

func (t *TokenType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	switch s {
	case "access":
		*t = TokenTypeAccess
	case "refresh":
		*t = TokenTypeRefresh
	default:
		return ErrInvalidTokenType
	}

	return nil
}

func (t TokenType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// Claims represents the JWT payload
type Claims struct {
	UserID string    `json:"user_id"`
	Type   TokenType `json:"type"` // "access" or "refresh"
	jwt.RegisteredClaims
}

// TokenPair contains both access and refresh tokens
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // seconds until access token expires
}

// Token expiry durations
const (
	AccessTokenDuration  = 15 * time.Minute
	RefreshTokenDuration = 7 * 24 * time.Hour // 7 days
)

// getSecretKey returns the JWT secret from environment or a default
func getSecretKey() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "your-super-secret-key-change-in-production"
	}
	return []byte(secret)
}

// GenerateTokenPair creates both access and refresh tokens for a user
func GenerateTokenPair(userID string) (*TokenPair, error) {
	accessToken, err := generateToken(userID, TokenTypeAccess, AccessTokenDuration)
	if err != nil {
		return nil, err
	}

	refreshToken, err := generateToken(userID, TokenTypeRefresh, RefreshTokenDuration)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(AccessTokenDuration.Seconds()),
	}, nil
}

// generateToken creates a JWT with the specified type and duration
func generateToken(userID string, tokenType TokenType, duration time.Duration) (string, error) {
	now := time.Now()
	claims := &Claims{
		UserID: userID,
		Type:   tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(getSecretKey())
}

// ValidateAccessToken validates an access token and returns claims
func ValidateAccessToken(tokenString string) (*Claims, error) {
	claims, err := validateToken(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.Type != TokenTypeAccess {
		return nil, ErrWrongType
	}

	return claims, nil
}

// ValidateRefreshToken validates a refresh token and returns claims
func ValidateRefreshToken(tokenString string) (*Claims, error) {
	claims, err := validateToken(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.Type != TokenTypeRefresh {
		return nil, ErrWrongType
	}

	return claims, nil
}

// validateToken parses and validates a JWT token
func validateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return getSecretKey(), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// RefreshAccessToken generates a new access token from a valid refresh token
func RefreshAccessToken(refreshTokenString string) (*TokenPair, error) {
	claims, err := ValidateRefreshToken(refreshTokenString)
	if err != nil {
		return nil, err
	}

	// Generate new access token only
	accessToken, err := generateToken(claims.UserID, TokenTypeAccess, AccessTokenDuration)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenString, // Return same refresh token
		ExpiresIn:    int64(AccessTokenDuration.Seconds()),
	}, nil
}
