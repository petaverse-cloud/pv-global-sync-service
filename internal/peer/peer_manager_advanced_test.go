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

func TestPeerManager_LastCheckSetOnHealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pm := NewPeerManager([]string{server.URL}, 100*time.Millisecond)

	// LastCheck should be zero before any health check
	pm.mu.RLock()
	if !pm.peers[0].LastCheck.IsZero() {
		t.Error("LastCheck should be zero before first health check")
	}
	pm.mu.RUnlock()

	// Perform health check
	beforeCheck := time.Now()
	pm.CheckHealth(context.Background())
	afterCheck := time.Now()

	// LastCheck should now be non-zero
	pm.mu.RLock()
	lastCheck := pm.peers[0].LastCheck
	pm.mu.RUnlock()

	if lastCheck.IsZero() {
		t.Fatal("LastCheck should not be zero after health check")
	}
	if lastCheck.Before(beforeCheck) || lastCheck.After(afterCheck) {
		t.Errorf("LastCheck %v should be between %v and %v", lastCheck, beforeCheck, afterCheck)
	}
}

func TestPeerManager_LastCheckSetOnFailedCheck(t *testing.T) {
	// Use an unreachable URL so the check fails
	pm := NewPeerManager([]string{"http://nonexistent-peer-xyz:19999"}, 100*time.Millisecond)

	pm.CheckHealth(context.Background())

	pm.mu.RLock()
	lastCheck := pm.peers[0].LastCheck
	pm.mu.RUnlock()

	if lastCheck.IsZero() {
		t.Fatal("LastCheck should be set even when health check fails")
	}
}

func TestPeerManager_SuccessfulCheckResetsFailCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pm := NewPeerManager([]string{server.URL}, 100*time.Millisecond)

	// Manually accumulate failures
	pm.MarkUnhealthy(server.URL)
	pm.MarkUnhealthy(server.URL)
	pm.MarkUnhealthy(server.URL)

	pm.mu.RLock()
	fcBefore := pm.peers[0].FailCount
	pm.mu.RUnlock()

	if fcBefore < 3 {
		t.Fatalf("expected FailCount >= 3, got %d", fcBefore)
	}

	// A successful health check should reset FailCount to 0
	pm.CheckHealth(context.Background())

	pm.mu.RLock()
	fcAfter := pm.peers[0].FailCount
	healthy := pm.peers[0].Healthy
	pm.mu.RUnlock()

	if fcAfter != 0 {
		t.Errorf("FailCount after successful check = %d, want 0", fcAfter)
	}
	if !healthy {
		t.Error("peer should be healthy after successful check")
	}
}

func TestPeerManager_DuplicatePeerURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Add the same URL twice
	pm := NewPeerManager([]string{server.URL, server.URL, server.URL}, 100*time.Millisecond)

	// All three entries should be tracked separately
	if pm.PeerCount() != 3 {
		t.Fatalf("PeerCount() = %d, want 3 (duplicates should be tracked separately)", pm.PeerCount())
	}

	allPeers := pm.AllPeers()
	for i, url := range allPeers {
		if url != server.URL {
			t.Errorf("AllPeers()[%d] = %s, want %s", i, url, server.URL)
		}
	}

	// After health check, all should be healthy
	pm.CheckHealth(context.Background())
	healthy := pm.HealthyPeers()
	if len(healthy) != 3 {
		t.Errorf("HealthyPeers() = %d, want 3", len(healthy))
	}

	// Marking one unhealthy by URL should only affect the first match
	pm.MarkUnhealthy(server.URL)
	healthyAfter := pm.HealthyPeers()
	// Only first entry is marked unhealthy, so 2 remain healthy
	if len(healthyAfter) != 2 {
		t.Errorf("after MarkUnhealthy one entry, HealthyPeers() = %d, want 2", len(healthyAfter))
	}
}

func TestPeerManager_TrailingSlashBehavior(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Register peer without trailing slash
	pm := NewPeerManager([]string{server.URL}, 100*time.Millisecond)

	// CheckPeer with URL that has a trailing slash — should NOT match
	// because the stored URL doesn't have the trailing slash
	urlWithSlash := server.URL + "/"
	ok := pm.CheckPeer(context.Background(), urlWithSlash)
	if ok {
		t.Log("CheckPeer matched URL with trailing slash — URL normalization may be in effect")
	}

	// The original URL without slash should definitely match
	ok2 := pm.CheckPeer(context.Background(), server.URL)
	if !ok2 {
		t.Error("CheckPeer() = false for exact URL match, want true")
	}
}

func TestPeerManager_MarkUnhealthyUnknownURLDoesNotChangeExistingPeers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pm := NewPeerManager([]string{server.URL}, 100*time.Millisecond)

	// Mark an unknown URL as unhealthy — should be a no-op
	pm.MarkUnhealthy("http://unknown-peer:8080")

	// Original peer should still be healthy
	healthy := pm.HealthyPeers()
	if len(healthy) != 1 || healthy[0] != server.URL {
		t.Error("MarkUnhealthy on unknown URL should not affect existing peers")
	}
}

func TestPeerManager_StressTest_50Goroutines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pm := NewPeerManager([]string{server.URL, "http://peer2:8080", "http://peer3:8080", "http://peer4:8080"}, 200*time.Millisecond)

	var wg sync.WaitGroup
	const goroutines = 80

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			switch id % 6 {
			case 0:
				pm.CheckHealth(context.Background())
			case 1:
				_ = pm.HealthyPeers()
			case 2:
				_ = pm.AllPeers()
			case 3:
				pm.MarkUnhealthy(server.URL)
			case 4:
				pm.MarkHealthy(server.URL)
			case 5:
				_ = pm.PeerCount()
			}
		}(i)
	}

	wg.Wait()
	// If we reach here without panic or data race, the test passes
	count := pm.PeerCount()
	if count != 4 {
		t.Errorf("PeerCount() after stress = %d, want 4", count)
	}
}

func TestPeerManager_StressTest_RapidCheckPeer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pm := NewPeerManager([]string{server.URL, "http://peer2:8080"}, 200*time.Millisecond)

	var wg sync.WaitGroup
	const goroutines = 60

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Mix of CheckPeer calls on known and unknown URLs
			if id%2 == 0 {
				_ = pm.CheckPeer(context.Background(), server.URL)
			} else {
				_ = pm.CheckPeer(context.Background(), "http://unknown:9999")
			}
		}(i)
	}

	wg.Wait()
	// No panic means concurrent CheckPeer is safe
}
