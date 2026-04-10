package redis

import "testing"

func TestFeedCacheKey(t *testing.T) {
	tests := []struct {
		userID   int64
		feedType string
		want     string
	}{
		{1, "following", "user:feed:1:following"},
		{42, "global", "user:feed:42:global"},
		{999, "trending", "user:feed:999:trending"},
	}

	for _, tt := range tests {
		got := FeedCacheKey(tt.userID, tt.feedType)
		if got != tt.want {
			t.Errorf("FeedCacheKey(%d, %q) = %q, want %q", tt.userID, tt.feedType, got, tt.want)
		}
	}
}

func TestPostCacheKey(t *testing.T) {
	tests := []struct {
		postID int64
		want   string
	}{
		{1, "post:1"},
		{12345, "post:12345"},
	}

	for _, tt := range tests {
		got := PostCacheKey(tt.postID)
		if got != tt.want {
			t.Errorf("PostCacheKey(%d) = %q, want %q", tt.postID, got, tt.want)
		}
	}
}

func TestConfigAddr(t *testing.T) {
	cfg := Config{Host: "redis.example.com", Port: 6380}
	want := "redis.example.com:6380"
	if got := cfg.Addr(); got != want {
		t.Errorf("Config.Addr() = %q, want %q", got, want)
	}
}
