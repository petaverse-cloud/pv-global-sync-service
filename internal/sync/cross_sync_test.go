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
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

func newTestLogger() *logger.Logger {
	l, _ := logger.New("debug", "console")
	return l
}

func testEvent(id string) *model.CrossRegionSyncEvent {
	return &model.CrossRegionSyncEvent{
		EventID:      id,
		EventType:    model.EventTypePostCreated,
		SourceRegion: model.RegionNA,
		TargetRegion: model.RegionEU,
		Timestamp:    time.Now().UnixMilli(),
		Payload: model.EventPayload{
			PostID:       100,
			AuthorID:     1,
			AuthorRegion: model.RegionNA,
			Visibility:   model.VisibilityGlobal,
			Content:      "Test post",
		},
		Metadata: model.EventMetadata{
			GDPRCompliant: true,
			UserConsent:   true,
			DataCategory:  model.DataCategoryUGC,
			CrossBorderOK: true,
		},
	}
}

func TestBroadcast_NoPeers(t *testing.T) {
	pm := peer.NewPeerManager([]string{}, 100*time.Millisecond)
	svc := NewCrossSyncService(pm, 100*time.Millisecond, newTestLogger())

	count := svc.Broadcast(context.Background(), testEvent("no_peers"))
	if count != 0 {
		t.Errorf("Broadcast() = %d, want 0", count)
	}
}

func TestBroadcast_SinglePeer(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/cross-sync" {
			received.Add(1)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	pm := peer.NewPeerManager([]string{server.URL}, 100*time.Millisecond)
	svc := NewCrossSyncService(pm, 100*time.Millisecond, newTestLogger())

	count := svc.Broadcast(context.Background(), testEvent("single"))
	if count != 1 {
		t.Errorf("Broadcast() = %d, want 1", count)
	}
	if received.Load() != 1 {
		t.Errorf("peer received %d requests, want 1", received.Load())
	}
}

func TestBroadcast_MultiplePeers(t *testing.T) {
	var receivedCount atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/cross-sync" {
			receivedCount.Add(1)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	server1 := httptest.NewServer(handler)
	defer server1.Close()
	server2 := httptest.NewServer(handler)
	defer server2.Close()

	pm := peer.NewPeerManager([]string{server1.URL, server2.URL}, 100*time.Millisecond)
	svc := NewCrossSyncService(pm, 100*time.Millisecond, newTestLogger())

	count := svc.Broadcast(context.Background(), testEvent("multi"))
	if count != 2 {
		t.Errorf("Broadcast() = %d, want 2", count)
	}
	if receivedCount.Load() != 2 {
		t.Errorf("total received = %d, want 2", receivedCount.Load())
	}
}

func TestBroadcast_UnhealthyPeerSkipped(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/cross-sync" {
			received.Add(1)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create with one healthy and one unhealthy peer
	pm := peer.NewPeerManager([]string{server.URL, "http://dead-peer:9999"}, 100*time.Millisecond)
	svc := NewCrossSyncService(pm, 100*time.Millisecond, newTestLogger())

	// Check health to mark dead peer as unhealthy
	pm.CheckHealth(context.Background())

	count := svc.Broadcast(context.Background(), testEvent("unhealthy"))
	// Only the healthy peer should receive the broadcast
	if count != 1 {
		t.Errorf("Broadcast() = %d, want 1", count)
	}
	if received.Load() != 1 {
		t.Errorf("peer received %d requests, want 1", received.Load())
	}
}

func TestBroadcast_Idempotent(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/cross-sync" {
			received.Add(1)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	pm := peer.NewPeerManager([]string{server.URL}, 100*time.Millisecond)
	svc := NewCrossSyncService(pm, 100*time.Millisecond, newTestLogger())

	eventID := "idempotent_test"
	event := testEvent(eventID)

	// First broadcast
	count1 := svc.Broadcast(context.Background(), event)
	if count1 != 1 {
		t.Errorf("1st Broadcast() = %d, want 1", count1)
	}

	// Second broadcast with same event ID should be skipped
	count2 := svc.Broadcast(context.Background(), event)
	if count2 != 0 {
		t.Errorf("2nd Broadcast() = %d, want 0 (idempotent)", count2)
	}

	if received.Load() != 1 {
		t.Errorf("peer received %d requests, want 1 (idempotent)", received.Load())
	}
}

func TestBroadcast_Reset(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/cross-sync" {
			received.Add(1)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	pm := peer.NewPeerManager([]string{server.URL}, 100*time.Millisecond)
	svc := NewCrossSyncService(pm, 100*time.Millisecond, newTestLogger())

	event := testEvent("reset_test")

	// First broadcast
	count1 := svc.Broadcast(context.Background(), event)
	if count1 != 1 {
		t.Errorf("1st Broadcast() = %d, want 1", count1)
	}

	// Reset and broadcast again
	svc.Reset()
	count2 := svc.Broadcast(context.Background(), event)
	if count2 != 1 {
		t.Errorf("after Reset, Broadcast() = %d, want 1", count2)
	}

	if received.Load() != 2 {
		t.Errorf("peer received %d requests, want 2 (after reset)", received.Load())
	}
}

func TestBroadcast_ServerErrorMarksUnhealthy(t *testing.T) {
	// Server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	pm := peer.NewPeerManager([]string{server.URL}, 100*time.Millisecond)
	svc := NewCrossSyncService(pm, 100*time.Millisecond, newTestLogger())

	count := svc.Broadcast(context.Background(), testEvent("server_error"))
	if count != 0 {
		t.Errorf("Broadcast() = %d, want 0 (500 response)", count)
	}

	// Peer should now be marked unhealthy
	healthy := pm.HealthyPeers()
	if len(healthy) != 0 {
		t.Errorf("after 500, healthy peers = %d, want 0", len(healthy))
	}
}
