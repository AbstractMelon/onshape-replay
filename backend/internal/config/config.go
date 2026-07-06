package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	// OAuth2 credentials for Onshape
	OAuthClientID     string
	OAuthClientSecret string
	OAuthCallbackURL  string

	// HTTP server
	Port string

	// Storage
	StorageRoot string

	// FFmpeg binary path
	FFmpegPath string

	// CORS allowed origins, comma-separated. Use * for development.
	AllowedOrigins []string
}

// Load reads configuration from environment variables.
// All required variables must be present or Load returns an error.
func Load() (*Config, error) {
	c := &Config{
		OAuthClientID:     os.Getenv("OAUTH_CLIENT_ID"),
		OAuthClientSecret: os.Getenv("OAUTH_CLIENT_SECRET"),
		OAuthCallbackURL:  os.Getenv("OAUTH_CALLBACK_URL"),
		Port:              envOr("PORT", "8080"),
		StorageRoot:       envOr("STORAGE_ROOT", "./storage"),
		FFmpegPath:        envOr("FFMPEG_PATH", "ffmpeg"),
		AllowedOrigins:    splitComma(envOr("ALLOWED_ORIGINS", "*")),
	}

	var missing []string
	if c.OAuthClientID == "" {
		missing = append(missing, "OAUTH_CLIENT_ID")
	}
	if c.OAuthClientSecret == "" {
		missing = append(missing, "OAUTH_CLIENT_SECRET")
	}
	if c.OAuthCallbackURL == "" {
		missing = append(missing, "OAUTH_CALLBACK_URL")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return c, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
