package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port               string
	JwtSecret          string
	PostgreSQLUrl      string
	RedisUrl           string
	DemoMode           bool
	MaxConcurrentQueue uint64
	WhitelistTTL       time.Duration
	HeartbeatTTL       time.Duration
	QueueTTL           time.Duration
	PromoteInterval    time.Duration
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	cfg := &Config{
		Port:               getEnv("PORT", "8000"),
		JwtSecret:          getEnv("JWT_SECRET", ""),
		PostgreSQLUrl:      getEnv("POSTGRESQL_URL", ""),
		RedisUrl:           getEnv("REDIS_URL", ""),
		DemoMode:           getEnvBool("DEMO_MODE", false),
		MaxConcurrentQueue: getEnvUInt64("MAX_CONCURRENT_QUEUE", 10),
		WhitelistTTL:       getEnvDuration("WHITELIST_TTL", 15*time.Minute),
		HeartbeatTTL:       getEnvDuration("HEARTBEAT_TTL", 5*time.Minute),
		QueueTTL:           getEnvDuration("QUEUE_TTL", 5*time.Minute),
		PromoteInterval:    getEnvDuration("PROMOTE_INTERVAL", 1*time.Second),
	}

	if err := mustNotEmptyEnv("JWT_SECRET", cfg.JwtSecret); err != nil {
		return nil, err
	}
	if err := mustNotEmptyEnv("POSTGRESQL_URL", cfg.PostgreSQLUrl); err != nil {
		return nil, err
	}
	if err := mustNotEmptyEnv("REDIS_URL", cfg.RedisUrl); err != nil {
		return nil, err
	}
	if err := mustAtLeast("WHITELIST_TTL", cfg.WhitelistTTL, time.Second); err != nil {
		return nil, err
	}
	if err := mustAtLeast("HEARTBEAT_TTL", cfg.HeartbeatTTL, time.Second); err != nil {
		return nil, err
	}
	if err := mustAtLeast("QUEUE_TTL", cfg.QueueTTL, time.Second); err != nil {
		return nil, err
	}
	if err := mustAtLeast("PROMOTE_INTERVAL", cfg.PromoteInterval, time.Millisecond); err != nil {
		return nil, err
	}

	return cfg, nil
}

func mustAtLeast(name string, value time.Duration, min time.Duration) error {
	if value < min {
		return fmt.Errorf("%s must be at least %s, got %s", name, min, value)
	}

	return nil
}

func mustNotEmptyEnv(name string, value string) error {
	if value == "" {
		return fmt.Errorf("%s must be set", name)
	}

	return nil
}

func getEnv(key string, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}

	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		val, err := strconv.ParseBool(v)
		if err != nil {
			return fallback
		}
		return val
	}

	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		val, err := time.ParseDuration(v)
		if err != nil {
			log.Printf("%s=%q is not a valid duration, falling back to %s", key, v, fallback)
			return fallback
		}
		return val
	}

	return fallback
}

func getEnvUInt64(key string, fallback uint64) uint64 {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		val, err := strconv.ParseUint(v, 10, 0)
		if err != nil {
			return fallback
		}
		return val
	}

	return fallback
}
