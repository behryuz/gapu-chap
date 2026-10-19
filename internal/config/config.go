package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	HTTPAddr        string
	ShutdownTimeout time.Duration
	DatabaseURL     string
	JWTSecret       string
	JWTTTL          time.Duration
	MigrateOnStart  bool
}

func Load() (*Config, error) {
	shutdownTimeout, err := time.ParseDuration(getEnv("SHUTDOWN_TIMEOUT", "10s"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse SHUTDOWN_TIMEOUT: %w", err)
	}

	databaseURL, err := requireEnv("DATABASE_URL")
	if err != nil {
		return nil, err
	}

	jwtSecret, err := requireEnv("JWT_SECRET")
	if err != nil {
		return nil, err
	}

	if len(jwtSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}

	jwtTTL, err := time.ParseDuration(getEnv("JWT_TTL", "24h"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT_TTL: %w", err)
	}

	return &Config{
		HTTPAddr:        getEnv("HTTP_ADDR", ":8081"),
		ShutdownTimeout: shutdownTimeout,
		DatabaseURL:     databaseURL,
		JWTSecret:       jwtSecret,
		JWTTTL:          jwtTTL,
		MigrateOnStart:  getEnv("MIGRATE_ON_START", "false") == "true",
	}, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func requireEnv(key string) (string, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return "", fmt.Errorf("required env variable %s is not set", key)
	}
	return v, nil
}
