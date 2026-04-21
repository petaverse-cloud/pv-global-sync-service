package peer

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// PeerStatus tracks the health of a peer.
type PeerStatus struct {
	URL       string
	Healthy   bool
	LastCheck time.Time
	FailCount int
}

// PeerManager manages a list of peer Global Sync services with health checking.
type PeerManager struct {
	mu      sync.RWMutex
	peers   []PeerStatus
	client  *http.Client
	timeout time.Duration
}

// NewPeerManager creates a PeerManager from a list of peer URLs.
func NewPeerManager(urls []string, timeout time.Duration) *PeerManager {
	pm := &PeerManager{
		client: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}
	for _, u := range urls {
		pm.peers = append(pm.peers, PeerStatus{
			URL:     u,
			Healthy: true, // Assume healthy until first check fails
		})
	}
	return pm
}

// HealthyPeers returns URLs of peers that are currently healthy.
func (pm *PeerManager) HealthyPeers() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var result []string
	for _, p := range pm.peers {
		if p.Healthy {
			result = append(result, p.URL)
		}
	}
	return result
}

// AllPeers returns all peer URLs regardless of health.
func (pm *PeerManager) AllPeers() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]string, len(pm.peers))
	for i, p := range pm.peers {
		result[i] = p.URL
	}
	return result
}

// CheckHealth checks all peers and updates their status.
func (pm *PeerManager) CheckHealth(ctx context.Context) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for i := range pm.peers {
		pm.checkPeerHealth(ctx, &pm.peers[i])
	}
}

// CheckPeer checks a single peer by URL.
func (pm *PeerManager) CheckPeer(ctx context.Context, url string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for i := range pm.peers {
		if pm.peers[i].URL == url {
			pm.checkPeerHealth(ctx, &pm.peers[i])
			return pm.peers[i].Healthy
		}
	}
	return false
}

// MarkUnhealthy marks a peer as unhealthy and increments fail count.
func (pm *PeerManager) MarkUnhealthy(url string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for i := range pm.peers {
		if pm.peers[i].URL == url {
			pm.peers[i].Healthy = false
			pm.peers[i].FailCount++
			pm.peers[i].LastCheck = time.Now().UTC()
			return
		}
	}
}

// MarkHealthy marks a peer as healthy and resets fail count.
func (pm *PeerManager) MarkHealthy(url string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for i := range pm.peers {
		if pm.peers[i].URL == url {
			pm.peers[i].Healthy = true
			pm.peers[i].FailCount = 0
			pm.peers[i].LastCheck = time.Now().UTC()
			return
		}
	}
}

// PeerCount returns the number of configured peers.
func (pm *PeerManager) PeerCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.peers)
}

func (pm *PeerManager) checkPeerHealth(ctx context.Context, p *PeerStatus) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/health", nil)
	if err != nil {
		p.Healthy = false
		p.FailCount++
		p.LastCheck = time.Now().UTC()
		return
	}

	resp, err := pm.client.Do(req)
	if err != nil {
		p.Healthy = false
		p.FailCount++
		p.LastCheck = time.Now().UTC()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		p.Healthy = true
		p.FailCount = 0
	} else {
		p.Healthy = false
		p.FailCount++
	}
	p.LastCheck = time.Now().UTC()
}
