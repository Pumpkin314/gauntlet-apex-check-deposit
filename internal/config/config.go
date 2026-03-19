// Package config centralizes magic numbers and tunable constants.
// Values are set from environment variables with sensible defaults.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all tunable runtime constants.
type Config struct {
	// Database connection retry.
	DBRetryCount int           // DB_RETRY_COUNT, default 30
	DBRetryDelay time.Duration // DB_RETRY_DELAY (e.g. "2s"), default 2s

	// Funding engine duplicate detection window.
	DupDetectionWindow time.Duration // DUP_DETECTION_WINDOW (e.g. "5m"), default 5m

	// HTTP multipart upload limit in bytes.
	MaxUploadBytes int64 // MAX_UPLOAD_BYTES, default 10485760 (10 MB)

	// SSE broadcaster tuning.
	SSEKeepaliveInterval   time.Duration // SSE_KEEPALIVE (e.g. "30s"), default 30s
	SSEChannelBuffer       int           // SSE_CHANNEL_BUFFER, default 64
	PGListenerMinReconnect time.Duration // PG_LISTENER_MIN_RECONNECT (e.g. "10s"), default 10s
	PGListenerMaxReconnect time.Duration // PG_LISTENER_MAX_RECONNECT (e.g. "1m"), default 1m
	PGListenerPingInterval time.Duration // PG_LISTENER_PING (e.g. "90s"), default 90s
}

// Load reads configuration from environment variables, falling back to defaults.
func Load() Config {
	return Config{
		DBRetryCount:           envInt("DB_RETRY_COUNT", 30),
		DBRetryDelay:           envDuration("DB_RETRY_DELAY", 2*time.Second),
		DupDetectionWindow:     envDuration("DUP_DETECTION_WINDOW", 5*time.Minute),
		MaxUploadBytes:         envInt64("MAX_UPLOAD_BYTES", 10<<20),
		SSEKeepaliveInterval:   envDuration("SSE_KEEPALIVE", 30*time.Second),
		SSEChannelBuffer:       envInt("SSE_CHANNEL_BUFFER", 64),
		PGListenerMinReconnect: envDuration("PG_LISTENER_MIN_RECONNECT", 10*time.Second),
		PGListenerMaxReconnect: envDuration("PG_LISTENER_MAX_RECONNECT", time.Minute),
		PGListenerPingInterval: envDuration("PG_LISTENER_PING", 90*time.Second),
	}
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
