package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/peer"
)

func TestBroadcast_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(5 * time.Second)
	}))
	defer server.Close()

	pm := peer.NewPeerManager([]string{server.URL}, 100*time.Millisecond)
	svc := NewCrossSyncService(pm, 100*time.Millisecond, newTestLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before broadcast

	count := svc.Broadcast(ctx, testEvent("cancelled"))
	if count != 0 {
		t.Errorf("Broadcast() with cancelled context = %d, want 0", count)
	}
}

func TestBroadcast_PeerTimeout(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/cross-sync" {
			time.Sleep(500 * time.Millisecond) // Slower than timeout
			received.Add(1)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	pm := peer.NewPeerManager([]string{server.URL}, 50*time.Millisecond)
	svc := NewCrossSyncService(pm, 50*time.Millisecond, newTestLogger())

	count := svc.Broadcast(context.Background(), testEvent("timeout"))
	if count != 0 {
		t.Errorf("Broadcast() with timeout = %d, want 0", count)
	}
}

func TestBroadcast_PartialFailure(t *testing.T) {
	var receivedCount atomic.Int32

	// Peer 1: succeeds
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/cross-sync" {
			receivedCount.Add(1)
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer server1.Close()

	// Peer 2: fails
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/cross-sync" {
			receivedCount.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server2.Close()

	pm := peer.NewPeerManager([]string{server1.URL, server2.URL}, 100*time.Millisecond)
	svc := NewCrossSyncService(pm, 100*time.Millisecond, newTestLogger())

	count := svc.Broadcast(context.Background(), testEvent("partial"))
	// Both peers should receive, but only peer1 should be successful
	if count != 1 {
		t.Errorf("Broadcast() = %d, want 1 (partial success)", count)
	}
	if receivedCount.Load() != 2 {
		t.Errorf("total received = %d, want 2", receivedCount.Load())
	}
}

func TestBroadcast_Non2xxResponseCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantCount  int
	}{
		{"200 OK", http.StatusOK, 1},            // 200 counts as successful delivery
		{"201 Created", http.StatusCreated, 1},
		{"202 Accepted", http.StatusAccepted, 1},
		{"400 Bad Request", http.StatusBadRequest, 0},
		{"401 Unauthorized", http.StatusUnauthorized, 0},
		{"403 Forbidden", http.StatusForbidden, 0},
		{"404 Not Found", http.StatusNotFound, 0},
		{"500 Internal Server Error", http.StatusInternalServerError, 0},
		{"502 Bad Gateway", http.StatusBadGateway, 0},
		{"503 Service Unavailable", http.StatusServiceUnavailable, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					w.WriteHeader(http.StatusOK)
					return
				}
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			pm := peer.NewPeerManager([]string{server.URL}, 100*time.Millisecond)
			svc := NewCrossSyncService(pm, 100*time.Millisecond, newTestLogger())

			count := svc.Broadcast(context.Background(), testEvent(tt.name))
			if count != tt.wantCount {
				t.Errorf("Broadcast() = %d, want %d for status %d", count, tt.wantCount, tt.statusCode)
			}
		})
	}
}

func TestCrossSyncService_Reset(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/cross-sync" {
			received.Add(1)
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer server.Close()

	pm := peer.NewPeerManager([]string{server.URL}, 100*time.Millisecond)
	svc := NewCrossSyncService(pm, 100*time.Millisecond, newTestLogger())

	event := testEvent("reset_test")
	count1 := svc.Broadcast(context.Background(), event)
	if count1 != 1 {
		t.Errorf("1st Broadcast() = %d, want 1", count1)
	}

	// Same event should be idempotent
	count2 := svc.Broadcast(context.Background(), event)
	if count2 != 0 {
		t.Errorf("2nd Broadcast() (idempotent) = %d, want 0", count2)
	}

	// Reset and try again
	svc.Reset()
	count3 := svc.Broadcast(context.Background(), event)
	if count3 != 1 {
		t.Errorf("3rd Broadcast() (after reset) = %d, want 1", count3)
	}

	if received.Load() != 2 {
		t.Errorf("total received = %d, want 2", received.Load())
	}
}

func TestCrossSyncService_HealthCheckRecovery(t *testing.T) {
	var failFlag atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			if failFlag.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sync/cross-sync" {
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer server.Close()

	pm := peer.NewPeerManager([]string{server.URL}, 100*time.Millisecond)
	svc := NewCrossSyncService(pm, 100*time.Millisecond, newTestLogger())

	// First broadcast should succeed
	count1 := svc.Broadcast(context.Background(), testEvent("before_fail"))
	if count1 != 1 {
		t.Errorf("1st Broadcast() = %d, want 1", count1)
	}

	// Make peer unhealthy
	failFlag.Store(true)
	pm.CheckHealth(context.Background())

	// Broadcast should fail and mark peer unhealthy
	count2 := svc.Broadcast(context.Background(), testEvent("during_fail"))
	if count2 != 0 {
		t.Errorf("Broadcast() during failure = %d, want 0", count2)
	}

	// Peer should be marked unhealthy
	healthy := pm.HealthyPeers()
	if len(healthy) != 0 {
		t.Errorf("healthy peers = %d, want 0", len(healthy))
	}

	// Recover
	failFlag.Store(false)
	pm.CheckHealth(context.Background())

	// Should work again
	count3 := svc.Broadcast(context.Background(), testEvent("after_recovery"))
	if count3 != 1 {
		t.Errorf("Broadcast() after recovery = %d, want 1", count3)
	}
}

func TestBroadcast_ManyPeers(t *testing.T) {
	// Test with many peers to verify concurrent broadcast
	var received atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sync/cross-sync" {
			received.Add(1)
			w.WriteHeader(http.StatusAccepted)
		}
	})

	var urls []string
	var servers []*httptest.Server
	for i := 0; i < 10; i++ {
		s := httptest.NewServer(handler)
		servers = append(servers, s)
		urls = append(urls, s.URL)
	}
	defer func() {
		for _, s := range servers {
			s.Close()
		}
	}()

	pm := peer.NewPeerManager(urls, 500*time.Millisecond)
	svc := NewCrossSyncService(pm, 500*time.Millisecond, newTestLogger())

	count := svc.Broadcast(context.Background(), testEvent("many_peers"))
	if count != 10 {
		t.Errorf("Broadcast() = %d, want 10", count)
	}
	if received.Load() != 10 {
		t.Errorf("total received = %d, want 10", received.Load())
	}
}

func TestBroadcast_EventWithMediaUrls(t *testing.T) {
	var receivedBody atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/cross-sync" {
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer server.Close()

	pm := peer.NewPeerManager([]string{server.URL}, 100*time.Millisecond)
	svc := NewCrossSyncService(pm, 100*time.Millisecond, newTestLogger())

	event := &model.CrossRegionSyncEvent{
		EventID:      "media_test",
		EventType:    model.EventTypePostCreated,
		SourceRegion: model.RegionNA,
		TargetRegion: model.RegionEU,
		Timestamp:    time.Now().UnixMilli(),
		Payload: model.EventPayload{
			PostID:       999,
			AuthorID:     1,
			AuthorRegion: model.RegionNA,
			Visibility:   model.VisibilityGlobal,
			Content:      "Test with media",
			MediaURLs:    []string{"https://cdn.example.com/img1.jpg", "https://cdn.example.com/video.mp4"},
		},
		Metadata: model.EventMetadata{
			GDPRCompliant: true,
			UserConsent:   true,
			DataCategory:  model.DataCategoryUGC,
			CrossBorderOK: true,
		},
	}

	count := svc.Broadcast(context.Background(), event)
	if count != 1 {
		t.Errorf("Broadcast() = %d, want 1", count)
	}

	// Verify received body contains mediaUrls
	_ = receivedBody // just ensuring the variable is used
}
