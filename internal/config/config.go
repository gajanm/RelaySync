package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Env                string
	HTTPAddr           string
	APITimeout         time.Duration
	APIKey             string
	RateLimitPerSecond float64
	RateLimitBurst     int
	PostgresURL        string
	RedisURL           string
	RedisStream        string
	RedisGroup         string
	RedisConsumer      string
	StreamBlock        time.Duration
	StreamBatchSize    int
	StreamBatchWindow  time.Duration
	LocationTTL        time.Duration
	MetricsPath        string
	FCMEnabled         bool
	FCMProjectID       string
	FCMTopicPrefix     string
	NotificationQueue  int
	NotifierWorkers    int
}

func Load() (Config, error) {
	cfg := Config{
		Env:                getEnv("APP_ENV", "dev"),
		HTTPAddr:           getEnv("HTTP_ADDR", ":8080"),
		APITimeout:         getDuration("HTTP_TIMEOUT", 5*time.Second),
		APIKey:             os.Getenv("API_KEY"),
		RateLimitPerSecond: getFloat("RATE_LIMIT_PER_SECOND", 20),
		RateLimitBurst:     getInt("RATE_LIMIT_BURST", 40),
		PostgresURL:        getEnv("POSTGRES_URL", "postgres://relaysync:relaysync@postgres:5432/relaysync?sslmode=disable"),
		RedisURL:           getEnv("REDIS_URL", "redis://redis:6379/0"),
		RedisStream:        getEnv("REDIS_STREAM", "relaysync:locations"),
		RedisGroup:         getEnv("REDIS_GROUP", "relaysync-consumers"),
		RedisConsumer:      getEnv("REDIS_CONSUMER", "consumer-1"),
		StreamBlock:        getDuration("STREAM_BLOCK", 2*time.Second),
		StreamBatchSize:    getInt("STREAM_BATCH_SIZE", 200),
		StreamBatchWindow:  getDuration("STREAM_BATCH_WINDOW", 500*time.Millisecond),
		LocationTTL:        getDuration("LOCATION_TTL", 30*time.Second),
		MetricsPath:        getEnv("METRICS_PATH", "/metrics"),
		FCMEnabled:         getBool("FCM_ENABLED", false),
		FCMProjectID:       getEnv("FCM_PROJECT_ID", ""),
		FCMTopicPrefix:     getEnv("FCM_TOPIC_PREFIX", "courier-"),
		NotificationQueue:  getInt("NOTIFICATION_QUEUE", 1000),
		NotifierWorkers:    getInt("NOTIFIER_WORKERS", 4),
	}

	if cfg.APIKey == "" {
		return cfg, errors.New("API_KEY must be set")
	}

	return cfg, nil
}

func getEnv(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

func getInt(key string, def int) int {
	if val := os.Getenv(key); val != "" {
		parsed, err := strconv.Atoi(val)
		if err == nil {
			return parsed
		}
	}
	return def
}

func getFloat(key string, def float64) float64 {
	if val := os.Getenv(key); val != "" {
		parsed, err := strconv.ParseFloat(val, 64)
		if err == nil {
			return parsed
		}
	}
	return def
}

func getBool(key string, def bool) bool {
	if val := os.Getenv(key); val != "" {
		parsed, err := strconv.ParseBool(val)
		if err == nil {
			return parsed
		}
	}
	return def
}

func getDuration(key string, def time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		parsed, err := time.ParseDuration(val)
		if err == nil {
			return parsed
		}
	}
	return def
}

func (c Config) String() string {
	return fmt.Sprintf("env=%s http=%s redis=%s postgres=%s stream=%s group=%s", c.Env, c.HTTPAddr, c.RedisURL, c.PostgresURL, c.RedisStream, c.RedisGroup)
}
