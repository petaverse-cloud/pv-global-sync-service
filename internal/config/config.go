package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for the Global Sync Service
type Config struct {
	// App
	Version     string
	Environment string
	Region      string // "eu" or "na"
	LogLevel    string
	LogFormat   string // "json" or "console"
	HTTPPort    int

	// PostgreSQL - Regional DB
	RegionalDBHost     string
	RegionalDBPort     int
	RegionalDBUser     string
	RegionalDBPassword string
	RegionalDBName     string
	RegionalDBSSLMode  string

	// PostgreSQL - Global Index DB
	GlobalIndexDBHost     string
	GlobalIndexDBPort     int
	GlobalIndexDBUser     string
	GlobalIndexDBPassword string
	GlobalIndexDBName     string
	GlobalIndexDBSSLMode  string

	// Redis
	RedisHost     string
	RedisPort     int
	RedisPassword string
	RedisDB       int

	// RocketMQ
	RocketMQNameServer string
	RocketMQGroupID    string
	RocketMQTopic      string
	RocketMQConsumer   string

	// Cross-region sync (multi-cluster)
	CrossSyncPeerURL  string          // Deprecated: use CrossSyncPeerURLs
	CrossSyncPeerURLs []string        // Comma-separated peer URLs (e.g. "https://sea-sync.example.com,https://eu-sync.example.com")
	CrossSyncTimeout  time.Duration

	// Feed generation
	FeedPushThreshold int // follower count threshold for push vs pull mode
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Version:     getEnv("APP_VERSION", "0.1.0"),
		Environment: getEnv("APP_ENV", "development"),
		Region:      getEnv("REGION", "na"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
		LogFormat:   getEnv("LOG_FORMAT", "json"),
		HTTPPort:    getEnvInt("HTTP_PORT", 8080),

		// Regional DB
		RegionalDBHost:     getEnv("REGIONAL_DB_HOST", "localhost"),
		RegionalDBPort:     getEnvInt("REGIONAL_DB_PORT", 5432),
		RegionalDBUser:     getEnv("REGIONAL_DB_USER", "postgres"),
		RegionalDBPassword: getEnv("REGIONAL_DB_PASSWORD", ""),
		RegionalDBName:     getEnv("REGIONAL_DB_NAME", "wigowago_regional"),
		RegionalDBSSLMode:  getEnv("REGIONAL_DB_SSL_MODE", "disable"),

		// Global Index DB
		GlobalIndexDBHost:     getEnv("GLOBAL_INDEX_DB_HOST", "localhost"),
		GlobalIndexDBPort:     getEnvInt("GLOBAL_INDEX_DB_PORT", 5433),
		GlobalIndexDBUser:     getEnv("GLOBAL_INDEX_DB_USER", "postgres"),
		GlobalIndexDBPassword: getEnv("GLOBAL_INDEX_DB_PASSWORD", ""),
		GlobalIndexDBName:     getEnv("GLOBAL_INDEX_DB_NAME", "wigowago_global_index"),
		GlobalIndexDBSSLMode:  getEnv("GLOBAL_INDEX_DB_SSL_MODE", "disable"),

		// Redis
		RedisHost:     getEnv("REDIS_HOST", "localhost"),
		RedisPort:     getEnvInt("REDIS_PORT", 6379),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),

		// RocketMQ
		RocketMQNameServer: getEnv("ROCKETMQ_NAME_SERVER", "localhost:9876"),
		RocketMQGroupID:    getEnv("ROCKETMQ_GROUP_ID", "global-sync-group"),
		RocketMQTopic:      getEnv("ROCKETMQ_TOPIC", "sync-events"),
		RocketMQConsumer:   getEnv("ROCKETMQ_CONSUMER", "sync-consumer"),

		// Cross-region sync
		CrossSyncPeerURL:  getEnv("CROSS_SYNC_PEER_URL", ""),
		CrossSyncPeerURLs: parsePeerURLs(getEnv("PEER_URLS", "")),
		CrossSyncTimeout:  getEnvDuration("CROSS_SYNC_TIMEOUT", 10*time.Second),

		// Feed generation
		FeedPushThreshold: getEnvInt("FEED_PUSH_THRESHOLD", 1000),
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// Validate checks required configuration
func (c *Config) Validate() error {
	if c.Region != "eu" && c.Region != "na" {
		return fmt.Errorf("REGION must be 'eu' or 'na', got '%s'", c.Region)
	}
	return nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal
	}
	return d
}

// parsePeerURLs parses a comma-separated list of peer URLs.
func parsePeerURLs(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var urls []string
	for _, u := range strings.Split(raw, ",") {
		u = strings.TrimSpace(u)
		if u != "" {
			urls = append(urls, u)
		}
	}
	return urls
}
