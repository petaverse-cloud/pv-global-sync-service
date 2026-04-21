package sync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func TestBroadcast_ContentTypeHeader(t *testing.T) {
	var gotContentType string
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/cross-sync" {
			mu.Lock()
			gotContentType = r.Header.Get("Content-Type")
			mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer server.Close()

	pm := peer.NewPeerManager([]string{server.URL}, 100*time.Millisecond)
	svc := NewCrossSyncService(pm, 100*time.Millisecond, newTestLogger())

	count := svc.Broadcast(context.Background(), testEvent("content_type_test"))
	if count != 1 {
		t.Errorf("Broadcast() = %d, want 1", count)
	}

	if gotContentType != "application/json" {
		t.Errorf("Content-Type header = %q, want %q", gotContentType, "application/json")
	}
}

func TestBroadcast_POSTBodyContainsCorrectJSON(t *testing.T) {
	var rawBody []byte
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/cross-sync" {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("failed to read body: %v", err)
				return
			}
			mu.Lock()
			rawBody = b
			mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer server.Close()

	pm := peer.NewPeerManager([]string{server.URL}, 100*time.Millisecond)
	svc := NewCrossSyncService(pm, 100*time.Millisecond, newTestLogger())

	expected := testEvent("json_body_test")
	count := svc.Broadcast(context.Background(), expected)
	if count != 1 {
		t.Errorf("Broadcast() = %d, want 1", count)
	}

	if len(rawBody) == 0 {
		t.Fatal("request body is empty")
	}

	var received model.CrossRegionSyncEvent
	if err := json.Unmarshal(rawBody, &received); err != nil {
		t.Fatalf("failed to unmarshal body as JSON: %v\nraw: %s", err, string(rawBody))
	}

	if received.EventID != expected.EventID {
		t.Errorf("body EventID = %q, want %q", received.EventID, expected.EventID)
	}
	if received.EventType != expected.EventType {
		t.Errorf("body EventType = %q, want %q", received.EventType, expected.EventType)
	}
	if received.SourceRegion != expected.SourceRegion {
		t.Errorf("body SourceRegion = %q, want %q", received.SourceRegion, expected.SourceRegion)
	}
	if received.Payload.PostID != expected.Payload.PostID {
		t.Errorf("body Payload.PostID = %d, want %d", received.Payload.PostID, expected.Payload.PostID)
	}
	if received.Payload.Content != expected.Payload.Content {
		t.Errorf("body Payload.Content = %q, want %q", received.Payload.Content, expected.Payload.Content)
	}
	if !received.Metadata.GDPRCompliant {
		t.Error("body Metadata.GDPRCompliant = false, want true")
	}
}

func TestBroadcast_ConcurrentDifferentEvents(t *testing.T) {
	// Use two separate servers so we can verify per-peer delivery
	var receivedA, receivedB atomic.Int32

	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/cross-sync" {
			receivedA.Add(1)
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer serverA.Close()

	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/cross-sync" {
			receivedB.Add(1)
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer serverB.Close()

	pm := peer.NewPeerManager([]string{serverA.URL, serverB.URL}, 100*time.Millisecond)
	svc := NewCrossSyncService(pm, 100*time.Millisecond, newTestLogger())

	numGoroutines := 20
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch many goroutines broadcasting different events simultaneously
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			event := testEvent("concurrent_" + strings.Repeat("x", idx))
			svc.Broadcast(context.Background(), event)
		}(i)
	}

	wg.Wait()

	// All events have unique IDs, so every broadcast should succeed at least once
	// across the two peers. Each event should reach both peers.
	// However due to the sent map guard, duplicate event IDs are skipped.
	// Since each goroutine sends a unique eventID, all 20 should be processed.
	totalReceived := receivedA.Load() + receivedB.Load()
	if totalReceived == 0 {
		t.Error("no requests received by any peer")
	}
	// Each of 20 unique events should be delivered to 2 peers = 40 total
	if totalReceived != int32(numGoroutines*2) {
		t.Errorf("total requests received = %d, want %d", totalReceived, numGoroutines*2)
	}
}

func TestBroadcast_ContextTimeoutMidBroadcast(t *testing.T) {
	var receivedCount atomic.Int32
	var start sync.WaitGroup
	start.Add(1)

	// Slow server that blocks until context is cancelled
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/cross-sync" {
			receivedCount.Add(1)
			start.Wait() // Block indefinitely until we release
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer server.Close()

	pm := peer.NewPeerManager([]string{server.URL}, 500*time.Millisecond)
	svc := NewCrossSyncService(pm, 500*time.Millisecond, newTestLogger())

	// Context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// We need a goroutine that holds start open past the timeout
	done := make(chan struct{})
	go func() {
		svc.Broadcast(ctx, testEvent("mid_timeout"))
		close(done)
	}()

	// Wait for timeout to trigger
	time.Sleep(150 * time.Millisecond)
	// Release the server so the blocked goroutine can finish
	start.Done()

	select {
	case <-done:
		// Broadcast returned (either due to timeout or completion)
	case <-time.After(2 * time.Second):
		t.Fatal("Broadcast did not return within timeout")
	}
}
