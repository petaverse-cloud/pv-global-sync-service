package peer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewPeerManager(t *testing.T) {
	t.Run("empty urls", func(t *testing.T) {
		pm := NewPeerManager([]string{}, 5*time.Second)
		if pm.PeerCount() != 0 {
			t.Errorf("PeerCount() = %d, want 0", pm.PeerCount())
		}
	})

	t.Run("single peer", func(t *testing.T) {
		urls := []string{"https://sea.example.com"}
		pm := NewPeerManager(urls, 5*time.Second)
		if pm.PeerCount() != 1 {
			t.Errorf("PeerCount() = %d, want 1", pm.PeerCount())
		}
		peers := pm.AllPeers()
		if peers[0] != "https://sea.example.com" {
			t.Errorf("AllPeers()[0] = %q, want %q", peers[0], "https://sea.example.com")
		}
	})

	t.Run("multiple peers", func(t *testing.T) {
		urls := []string{"https://sea.example.com", "https://eu.example.com", "https://us.example.com"}
		pm := NewPeerManager(urls, 5*time.Second)
		if pm.PeerCount() != 3 {
			t.Errorf("PeerCount() = %d, want 3", pm.PeerCount())
		}
	})
}

func TestHealthyPeers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pm := NewPeerManager([]string{server.URL, "http://nonexistent-peer:9999"}, 100*time.Millisecond)

	// Initially all peers are assumed healthy
	healthy := pm.HealthyPeers()
	if len(healthy) != 2 {
		t.Errorf("initial HealthyPeers len = %d, want 2", len(healthy))
	}

	// After health check, the nonexistent peer should be unhealthy
	pm.CheckHealth(context.Background())
	healthy = pm.HealthyPeers()
	if len(healthy) != 1 {
		t.Errorf("after check HealthyPeers len = %d, want 1", len(healthy))
	}
	if healthy[0] != server.URL {
		t.Errorf("healthy peer = %q, want %q", healthy[0], server.URL)
	}
}

func TestCheckPeer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	pm := NewPeerManager([]string{server.URL}, 100*time.Millisecond)

	ok := pm.CheckPeer(context.Background(), server.URL)
	if !ok {
		t.Error("CheckPeer() = false, want true for healthy peer")
	}

	ok = pm.CheckPeer(context.Background(), "http://nonexistent:9999")
	if ok {
		t.Error("CheckPeer() = true, want false for unknown peer")
	}
}

func TestMarkHealthyUnhealthy(t *testing.T) {
	url := "https://test.example.com"
	pm := NewPeerManager([]string{url}, 5*time.Second)

	pm.MarkUnhealthy(url)
	healthy := pm.HealthyPeers()
	if len(healthy) != 0 {
		t.Errorf("after MarkUnhealthy, HealthyPeers len = %d, want 0", len(healthy))
	}

	pm.MarkHealthy(url)
	healthy = pm.HealthyPeers()
	if len(healthy) != 1 {
		t.Errorf("after MarkHealthy, HealthyPeers len = %d, want 1", len(healthy))
	}
}

func TestPeerHealth_500Response(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	pm := NewPeerManager([]string{server.URL}, 100*time.Millisecond)
	pm.CheckHealth(context.Background())

	healthy := pm.HealthyPeers()
	if len(healthy) != 0 {
		t.Errorf("500 response: HealthyPeers len = %d, want 0", len(healthy))
	}
}

func TestConcurrentAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pm := NewPeerManager([]string{server.URL}, 100*time.Millisecond)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			pm.CheckHealth(context.Background())
			pm.HealthyPeers()
			pm.AllPeers()
			pm.MarkUnhealthy(server.URL)
			pm.MarkHealthy(server.URL)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
