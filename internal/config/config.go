package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Workers       int
	QueueSize     int
	BaseBackoff   time.Duration
	MaxBackoff    time.Duration
	ServerAddr    string
}

func LoadConfig() *Config {
	return &Config{
		Workers:     getEnvAsInt("WORKERS", 4),
		QueueSize:   getEnvAsInt("QUEUE_SIZE", 64),
		BaseBackoff: time.Duration(getEnvAsInt("BASE_BACKOFF_MS", 100)) * time.Millisecond,
		MaxBackoff:  time.Duration(getEnvAsInt("MAX_BACKOFF_MS", 10000)) * time.Millisecond,
		ServerAddr:  getEnvAsString("SERVER_ADDR", ":8080"),
	}
}

func getEnvAsString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if valueStr := os.Getenv(key); valueStr != "" {
		if value, err := strconv.Atoi(valueStr); err == nil {
			return value
		}
	}
	return defaultValue
}