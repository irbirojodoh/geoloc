package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	// ErrInvalidIDToken is returned when an ID token cannot be verified.
	ErrInvalidIDToken = errors.New("invalid ID token")
	// ErrTokenIssuer is returned when the token's issuer claim is unexpected.
	ErrTokenIssuer = errors.New("invalid token issuer")
	// ErrTokenAudience is returned when the token's audience does not match the app's client ID.
	ErrTokenAudience = errors.New("invalid token audience")
)

// SocialUser holds verified user info extracted from a social provider's ID token.
type SocialUser struct {
	Email     string
	FullName  string
	AvatarURL string
	Provider  string // "google" or "apple"
}

// ── Google Verification ──────────────────────────────────────────────────────

// VerifyGoogleIDToken verifies a Google ID token using Google's tokeninfo endpoint
// and extracts user information. For high-throughput production use, switch to
// local JWKS verification at https://www.googleapis.com/oauth2/v3/certs.
func VerifyGoogleIDToken(ctx context.Context, idToken, clientID string) (*SocialUser, error) {
	url := "https://oauth2.googleapis.com/tokeninfo?id_token=" + idToken

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Google tokeninfo request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to contact Google tokeninfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ErrInvalidIDToken
	}

	var claims struct {
		Email         string `json:"email"`
		EmailVerified string `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
		Aud           string `json:"aud"`
		Iss           string `json:"iss"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return nil, fmt.Errorf("failed to decode Google token claims: %w", err)
	}

	// Verify audience matches our registered client ID
	if claims.Aud != clientID {
		return nil, ErrTokenAudience
	}
	// Verify issuer
	if claims.Iss != "accounts.google.com" && claims.Iss != "https://accounts.google.com" {
		return nil, ErrTokenIssuer
	}
	if claims.EmailVerified != "true" {
		return nil, fmt.Errorf("Google account email is not verified")
	}

	return &SocialUser{
		Email:     claims.Email,
		FullName:  claims.Name,
		AvatarURL: claims.Picture,
		Provider:  "google",
	}, nil
}

// ── Apple Verification ───────────────────────────────────────────────────────

// appleJWKSCache stores Apple's public JWKS with a 24-hour in-memory cache.
var (
	appleJWKSCache     *appleJWKS
	appleJWKSCacheMu   sync.RWMutex
	appleJWKSCacheTime time.Time
)

type appleJWKS struct {
	Keys []appleJWK `json:"keys"`
}

type appleJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// fetchApplePublicKeys fetches and caches Apple's public JWKS.
// Keys are cached for 24 hours to avoid hammering Apple's endpoint on every login.
func fetchApplePublicKeys(ctx context.Context) (*appleJWKS, error) {
	appleJWKSCacheMu.RLock()
	if appleJWKSCache != nil && time.Since(appleJWKSCacheTime) < 24*time.Hour {
		keys := appleJWKSCache
		appleJWKSCacheMu.RUnlock()
		return keys, nil
	}
	appleJWKSCacheMu.RUnlock()

	appleJWKSCacheMu.Lock()
	defer appleJWKSCacheMu.Unlock()

	// Double-check under write lock in case another goroutine just refreshed it
	if appleJWKSCache != nil && time.Since(appleJWKSCacheTime) < 24*time.Hour {
		return appleJWKSCache, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://appleid.apple.com/auth/keys", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Apple JWKS request: %w", err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Apple JWKS: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Apple JWKS response: %w", err)
	}

	var jwks appleJWKS
	if err := json.Unmarshal(body, &jwks); err != nil {
		return nil, fmt.Errorf("failed to parse Apple JWKS: %w", err)
	}

	appleJWKSCache = &jwks
	appleJWKSCacheTime = time.Now()
	return appleJWKSCache, nil
}

// VerifyAppleIDToken verifies an Apple ID token using Apple's public JWKS
// and RS256 signature algorithm.
func VerifyAppleIDToken(ctx context.Context, idToken, clientID string) (*SocialUser, error) {
	// 1. Fetch Apple's public keys (cached 24h)
	jwks, err := fetchApplePublicKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get Apple public keys: %w", err)
	}

	// 2. Parse the unverified token header to find key ID (kid)
	unverified, _, err := new(jwt.Parser).ParseUnverified(idToken, jwt.MapClaims{})
	if err != nil {
		return nil, ErrInvalidIDToken
	}
	kid, ok := unverified.Header["kid"].(string)
	if !ok || kid == "" {
		return nil, fmt.Errorf("%w: missing kid in header", ErrInvalidIDToken)
	}

	// 3. Find matching Apple key by kid
	var matchingKey *appleJWK
	for i := range jwks.Keys {
		if jwks.Keys[i].Kid == kid {
			matchingKey = &jwks.Keys[i]
			break
		}
	}
	if matchingKey == nil {
		return nil, fmt.Errorf("no Apple public key found for kid=%s", kid)
	}

	// 4. Construct RSA public key from JWK components
	pubKey, err := appleJWKToRSAPublicKey(matchingKey)
	if err != nil {
		return nil, fmt.Errorf("failed to build RSA public key: %w", err)
	}

	// 5. Verify signature and parse claims
	claims := jwt.MapClaims{}
	_, err = jwt.ParseWithClaims(idToken, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return pubKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("Apple token verification failed: %w", err)
	}

	// 6. Validate issuer
	iss, _ := claims["iss"].(string)
	if iss != "https://appleid.apple.com" {
		return nil, ErrTokenIssuer
	}

	// 7. Validate audience (our app's client ID / bundle ID)
	aud, _ := claims["aud"].(string)
	if aud != clientID {
		return nil, ErrTokenAudience
	}

	// 8. Extract email
	email, _ := claims["email"].(string)
	if email == "" {
		return nil, fmt.Errorf("email claim missing from Apple ID token")
	}

	return &SocialUser{
		Email:    email,
		// Apple never provides an avatar URL.
		// Apple only provides the user's name on the very first sign-in, and only
		// via the client-side response (not the ID token). The handler receives it
		// in the request body for first-time users.
		Provider: "apple",
	}, nil
}

// appleJWKToRSAPublicKey decodes an Apple JWK into an *rsa.PublicKey.
func appleJWKToRSAPublicKey(key *appleJWK) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWK N component: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWK E component: %w", err)
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(new(big.Int).SetBytes(eBytes).Int64()),
	}, nil
}
