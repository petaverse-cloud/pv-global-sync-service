package handler

import (
	"testing"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/service"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

func TestFeedGenerator_New(t *testing.T) {
	log, _ := logger.New("warn", "console")
	// Just verify constructor doesn't panic with nil deps
	_ = service.NewFeedGenerator(nil, nil, nil, log, 1000)
}

func TestFeedTTLs(t *testing.T) {
	ttls := service.FeedTTLs()
	if ttls["following"] == 0 {
		t.Error("following TTL should not be zero")
	}
	if ttls["global"] == 0 {
		t.Error("global TTL should not be zero")
	}
	if ttls["trending"] == 0 {
		t.Error("trending TTL should not be zero")
	}
}
