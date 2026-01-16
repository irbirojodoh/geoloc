package auth

import (
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/apple"
	"github.com/markbates/goth/providers/google"
)

var OAUTH_ENVIRONMENT = []string{
	"GOOGLE_CLIENT_ID",
	"GOOGLE_CLIENT_SECRET",
	"GOOGLE_CALLBACK_URL",

	"APPLE_CLIENT_ID",
	"APPLE_CLIENT_SECRET",
	"APPLE_CALLBACK_URL",
}

func InitOAuth() {
	sessionSecret, ok := os.LookupEnv("SESSION_SECRET")
	if !ok {
		slog.Error("❌ SESSION_SECRET is not set for OAuth capabilities. Just generate a random string")
		os.Exit(1)
	}

	for _, env := range OAUTH_ENVIRONMENT {
		if _, ok := os.LookupEnv(env); !ok {
			slog.Warn("[OAUTH] Undefined", "variable", env)
		}
	}

	// 1. Setup Session Store (Required for OAuth state validation)
	store := sessions.NewCookieStore([]byte(sessionSecret))
	store.MaxAge(300) // 5 minutes max for the login flow
	store.Options.Path = "/"
	store.Options.HttpOnly = true
	store.Options.SameSite = http.SameSiteLaxMode

	// Only set Secure if in production (HTTPS)
	// If running on localhost (HTTP), Secure=true will prevent the cookie from being set.
	store.Options.Secure = os.Getenv("GIN_MODE") == "release"
	gothic.Store = store

	// This tells Goth to look for the "provider" value in the Context
	gothic.GetProviderName = func(req *http.Request) (string, error) {
		// Try to get provider from context (set by Gin middleware/handler)
		if provider, ok := req.Context().Value("provider").(string); ok {
			return provider, nil
		}
		// Fallback to query param
		provider := req.URL.Query().Get("provider")
		if provider != "" {
			return provider, nil
		}
		return "", errors.New("you must select a provider")
	}

	// 2. Initialize Providers
	goth.UseProviders(
		// Google Provider
		google.New(
			os.Getenv("GOOGLE_CLIENT_ID"),
			os.Getenv("GOOGLE_CLIENT_SECRET"),
			os.Getenv("GOOGLE_CALLBACK_URL"),
			"email", "profile",
		),

		// Apple Provider
		apple.New(
			os.Getenv("APPLE_CLIENT_ID"),
			os.Getenv("APPLE_CLIENT_SECRET"), // For Apple, this is the generated Client Secret JWT
			os.Getenv("APPLE_CALLBACK_URL"),
			nil,
			apple.ScopeName, apple.ScopeEmail,
		),
	)

	log.Println("✅ OAuth Providers Initialized: Google, Apple")
}
