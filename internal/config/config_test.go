package config

import (
	"os"
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		region  string
		wantErr bool
	}{
		{"valid eu", "eu", false},
		{"valid na", "na", false},
		{"invalid region", "asia", true},
		{"empty region", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{Region: tt.region}
			err := c.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Set minimal required env vars
	os.Setenv("REGION", "eu")
	os.Setenv("HTTP_PORT", "9999")
	os.Setenv("REGIONAL_DB_HOST", "db.example.com")
	os.Setenv("GLOBAL_INDEX_DB_HOST", "idx.example.com")
	os.Setenv("REDIS_HOST", "redis.example.com")
	os.Setenv("ROCKETMQ_NAME_SERVER", "mq.example.com:9876")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Region != "eu" {
		t.Errorf("Region = %q, want %q", cfg.Region, "eu")
	}
	if cfg.HTTPPort != 9999 {
		t.Errorf("HTTPPort = %d, want %d", cfg.HTTPPort, 9999)
	}
	if cfg.RegionalDBHost != "db.example.com" {
		t.Errorf("RegionalDBHost = %q, want %q", cfg.RegionalDBHost, "db.example.com")
	}

	// Clean up
	os.Unsetenv("REGION")
	os.Unsetenv("HTTP_PORT")
	os.Unsetenv("REGIONAL_DB_HOST")
	os.Unsetenv("GLOBAL_INDEX_DB_HOST")
	os.Unsetenv("REDIS_HOST")
	os.Unsetenv("ROCKETMQ_NAME_SERVER")
}

func TestLoadDefaults(t *testing.T) {
	os.Setenv("REGION", "na")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort default = %d, want %d", cfg.HTTPPort, 8080)
	}
	if cfg.RegionalDBPort != 5432 {
		t.Errorf("RegionalDBPort default = %d, want %d", cfg.RegionalDBPort, 5432)
	}
	if cfg.RedisPort != 6379 {
		t.Errorf("RedisPort default = %d, want %d", cfg.RedisPort, 6379)
	}
	if cfg.FeedPushThreshold != 1000 {
		t.Errorf("FeedPushThreshold default = %d, want %d", cfg.FeedPushThreshold, 1000)
	}

	os.Unsetenv("REGION")
}

func TestParsePeerURLs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", "", []string{}},
		{"single", "https://sea.example.com", []string{"https://sea.example.com"}},
		{"multiple", "https://sea.example.com,https://eu.example.com", []string{"https://sea.example.com", "https://eu.example.com"}},
		{"with spaces", " https://sea.example.com , https://eu.example.com ", []string{"https://sea.example.com", "https://eu.example.com"}},
		{"trailing comma", "https://sea.example.com,", []string{"https://sea.example.com"}},
		{"empty entries", ",,https://sea.example.com,,", []string{"https://sea.example.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePeerURLs(tt.raw)
			if len(got) != len(tt.want) {
				t.Errorf("parsePeerURLs(%q) len = %d, want %d", tt.raw, len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parsePeerURLs(%q)[%d] = %q, want %q", tt.raw, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestLoadPeerURLs(t *testing.T) {
	os.Setenv("REGION", "na")
	os.Setenv("REGIONAL_DB_HOST", "db.example.com")
	os.Setenv("GLOBAL_INDEX_DB_HOST", "idx.example.com")
	os.Setenv("REDIS_HOST", "redis.example.com")
	os.Setenv("ROCKETMQ_NAME_SERVER", "mq.example.com:9876")

	tests := []struct {
		name string
		val  string
		want []string
	}{
		{"none", "", []string{}},
		{"single", "https://sea.example.com", []string{"https://sea.example.com"}},
		{"multiple", "https://sea.example.com,https://eu.example.com", []string{"https://sea.example.com", "https://eu.example.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("PEER_URLS", tt.val)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if len(cfg.CrossSyncPeerURLs) != len(tt.want) {
				t.Errorf("PeerURLs len = %d, want %d", len(cfg.CrossSyncPeerURLs), len(tt.want))
			}
			for i := range tt.want {
				if cfg.CrossSyncPeerURLs[i] != tt.want[i] {
					t.Errorf("PeerURLs[%d] = %q, want %q", i, cfg.CrossSyncPeerURLs[i], tt.want[i])
				}
			}
		})
	}

	// Clean up
	os.Unsetenv("REGION")
	os.Unsetenv("REGIONAL_DB_HOST")
	os.Unsetenv("GLOBAL_INDEX_DB_HOST")
	os.Unsetenv("REDIS_HOST")
	os.Unsetenv("ROCKETMQ_NAME_SERVER")
	os.Unsetenv("PEER_URLS")
}

func TestInvalidHTTPPortFallback(t *testing.T) {
	os.Setenv("REGION", "eu")
	os.Setenv("HTTP_PORT", "not-a-number")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %d with invalid env, want default %d", cfg.HTTPPort, 8080)
	}

	os.Unsetenv("REGION")
	os.Unsetenv("HTTP_PORT")
}

func TestInvalidCrossSyncTimeoutFallback(t *testing.T) {
	os.Setenv("REGION", "eu")
	os.Setenv("CROSS_SYNC_TIMEOUT", "bogus")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CrossSyncTimeout != 10*time.Second {
		t.Errorf("CrossSyncTimeout = %v with invalid env, want default %v", cfg.CrossSyncTimeout, 10*time.Second)
	}

	os.Unsetenv("REGION")
	os.Unsetenv("CROSS_SYNC_TIMEOUT")
}

func TestValidateRegionEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		region  string
		wantErr bool
	}{
		{"sea invalid", "sea", true},
		{"us-west invalid", "us-west", true},
		{"uppercase EU invalid", "EU", true},
		{"uppercase NA invalid", "NA", true},
		{"mixed case Na invalid", "Na", true},
		{"Eu mixed case invalid", "Eu", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{Region: tt.region}
			err := c.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadDefaultsExceptRegion(t *testing.T) {
	os.Setenv("REGION", "eu")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify only REGION was custom, everything else is default
	if cfg.Region != "eu" {
		t.Errorf("Region = %q, want %q", cfg.Region, "eu")
	}
	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %d, want %d", cfg.HTTPPort, 8080)
	}
	if cfg.RegionalDBHost != "localhost" {
		t.Errorf("RegionalDBHost = %q, want %q", cfg.RegionalDBHost, "localhost")
	}
	if cfg.RegionalDBPort != 5432 {
		t.Errorf("RegionalDBPort = %d, want %d", cfg.RegionalDBPort, 5432)
	}
	if cfg.GlobalIndexDBHost != "localhost" {
		t.Errorf("GlobalIndexDBHost = %q, want %q", cfg.GlobalIndexDBHost, "localhost")
	}
	if cfg.GlobalIndexDBPort != 5433 {
		t.Errorf("GlobalIndexDBPort = %d, want %d", cfg.GlobalIndexDBPort, 5433)
	}
	if cfg.RedisHost != "localhost" {
		t.Errorf("RedisHost = %q, want %q", cfg.RedisHost, "localhost")
	}
	if cfg.RedisPort != 6379 {
		t.Errorf("RedisPort = %d, want %d", cfg.RedisPort, 6379)
	}
	if cfg.RocketMQNameServer != "localhost:9876" {
		t.Errorf("RocketMQNameServer = %q, want %q", cfg.RocketMQNameServer, "localhost:9876")
	}
	if cfg.CrossSyncTimeout != 10*time.Second {
		t.Errorf("CrossSyncTimeout = %v, want %v", cfg.CrossSyncTimeout, 10*time.Second)
	}
	if cfg.FeedPushThreshold != 1000 {
		t.Errorf("FeedPushThreshold = %d, want %d", cfg.FeedPushThreshold, 1000)
	}
	if cfg.Version != "0.1.0" {
		t.Errorf("Version = %q, want %q", cfg.Version, "0.1.0")
	}
	if cfg.Environment != "development" {
		t.Errorf("Environment = %q, want %q", cfg.Environment, "development")
	}

	os.Unsetenv("REGION")
}

func TestParsePeerURLsWhitespaceOnly(t *testing.T) {
	got := parsePeerURLs("   ")
	if len(got) != 0 {
		t.Errorf("parsePeerURLs(%q) = %v, want empty slice", "   ", got)
	}
}

func TestParsePeerURLsCommasOnly(t *testing.T) {
	got := parsePeerURLs(",,,")
	if len(got) != 0 {
		t.Errorf("parsePeerURLs(%q) = %v, want empty slice", ",,,", got)
	}
}

func TestParsePeerURLsWhitespaceAndCommas(t *testing.T) {
	got := parsePeerURLs("  ,  ,  ")
	if len(got) != 0 {
		t.Errorf("parsePeerURLs(%q) = %v, want empty slice", "  ,  ,  ", got)
	}
}
