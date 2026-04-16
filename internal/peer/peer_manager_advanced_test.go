package peer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestPeerManager_MarkUnhealthyAndRecovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	pm := NewPeerManager([]string{server.URL}, 100*time.Millisecond)

	// Initially healthy
	healthy := pm.HealthyPeers()
	if len(healthy) != 1 || healthy[0] != server.URL {
		t.Error("peer should be healthy initially")
	}

	// Manually mark unhealthy
	pm.MarkUnhealthy(server.URL)
	if len(pm.HealthyPeers()) != 0 {
		t.Error("peer should be unhealthy after MarkUnhealthy")
	}

	// CheckHealth should recover it (server is alive)
	pm.CheckHealth(context.Background())
	if len(pm.HealthyPeers()) != 1 {
		t.Error("peer should recover after health check")
	}
}

func TestPeerManager_MarkHealthyOnUnknownPeer(t *testing.T) {
	pm := NewPeerManager([]string{"http://peer1:8080"}, 100*time.Millisecond)

	// Should not panic
	pm.MarkHealthy("http://unknown-peer:8080")
	pm.MarkUnhealthy("http://unknown-peer:8080")
}

func TestPeerManager_AllPeersVsHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	pm := NewPeerManager([]string{server.URL, "http://dead-peer:9999"}, 100*time.Millisecond)

	// AllPeers should always return all configured peers
	if len(pm.AllPeers()) != 2 {
		t.Errorf("AllPeers() = %d, want 2", len(pm.AllPeers()))
	}

	// After health check, only healthy peer should be in HealthyPeers
	pm.CheckHealth(context.Background())
	healthy := pm.HealthyPeers()
	if len(healthy) != 1 {
		t.Errorf("HealthyPeers() = %d, want 1", len(healthy))
	}
	if healthy[0] != server.URL {
		t.Errorf("healthy peer = %s, want %s", healthy[0], server.URL)
	}
}

func TestPeerManager_ConcurrentAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pm := NewPeerManager([]string{server.URL, "http://peer2:8080", "http://peer3:8080"}, 500*time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pm.HealthyPeers()
			pm.AllPeers()
			pm.CheckHealth(context.Background())
			pm.MarkUnhealthy(server.URL)
			pm.MarkHealthy(server.URL)
			pm.PeerCount()
		}()
	}
	wg.Wait()
	// If we get here without panic, the test passes
}

func TestPeerManager_PeerCount(t *testing.T) {
	pm := NewPeerManager([]string{"http://a:8080", "http://b:8080", "http://c:8080"}, 100*time.Millisecond)
	if pm.PeerCount() != 3 {
		t.Errorf("PeerCount() = %d, want 3", pm.PeerCount())
	}

	pm2 := NewPeerManager([]string{}, 100*time.Millisecond)
	if pm2.PeerCount() != 0 {
		t.Errorf("PeerCount() empty = %d, want 0", pm2.PeerCount())
	}
}

func TestPeerManager_CheckHealth_ContextCancel(t *testing.T) {
	// Server that never responds (for timeout testing)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(5 * time.Second)
	}))
	defer server.Close()

	pm := NewPeerManager([]string{server.URL}, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	pm.CheckHealth(ctx)
	// Should not panic, peer should be marked unhealthy
	healthy := pm.HealthyPeers()
	if len(healthy) != 0 {
		t.Errorf("after context cancel, healthy peers = %d, want 0", len(healthy))
	}
}

func TestPeerManager_FailCountIncrements(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	pm := NewPeerManager([]string{server.URL}, 100*time.Millisecond)

	// Check health 3 times
	for i := 0; i < 3; i++ {
		pm.CheckHealth(context.Background())
	}

	pm.mu.RLock()
	failCount := pm.peers[0].FailCount
	pm.mu.RUnlock()

	if failCount < 3 {
		t.Errorf("FailCount = %d, want >= 3", failCount)
	}
}

func TestPeerManager_MarkHealthyResetsFailCount(t *testing.T) {
	pm := NewPeerManager([]string{"http://peer:8080"}, 100*time.Millisecond)

	pm.MarkUnhealthy("http://peer:8080")
	pm.MarkUnhealthy("http://peer:8080")
	pm.MarkUnhealthy("http://peer:8080")

	pm.MarkHealthy("http://peer:8080")

	pm.mu.RLock()
	failCount := pm.peers[0].FailCount
	healthy := pm.peers[0].Healthy
	pm.mu.RUnlock()

	if failCount != 0 {
		t.Errorf("FailCount after MarkHealthy = %d, want 0", failCount)
	}
	if !healthy {
		t.Error("peer should be healthy after MarkHealthy")
	}
}
