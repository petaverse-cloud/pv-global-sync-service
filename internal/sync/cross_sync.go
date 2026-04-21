package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/peer"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

// CrossSyncService broadcasts sync events to all peer Global Sync services.
type CrossSyncService struct {
	pm     *peer.PeerManager
	client *http.Client
	log    *logger.Logger
	mu     sync.RWMutex
	sent   map[string]bool // eventID -> sent (for idempotency tracking)
}

// NewCrossSyncService creates a new CrossSyncService.
func NewCrossSyncService(pm *peer.PeerManager, timeout time.Duration, log *logger.Logger) *CrossSyncService {
	return &CrossSyncService{
		pm: pm,
		client: &http.Client{
			Timeout: timeout,
		},
		log:  log,
		sent: make(map[string]bool),
	}
}

// Broadcast sends the event to all healthy peers.
// Returns the count of successful deliveries.
func (s *CrossSyncService) Broadcast(ctx context.Context, event *model.CrossRegionSyncEvent) int {
	s.mu.Lock()
	if s.sent[event.EventID] {
		s.mu.Unlock()
		s.log.Debug("Event already broadcast, skipping",
			logger.String("event_id", event.EventID))
		return 0
	}
	s.sent[event.EventID] = true
	s.mu.Unlock()

	peers := s.pm.HealthyPeers()
	if len(peers) == 0 {
		s.log.Warn("No healthy peers to broadcast to",
			logger.String("event_id", event.EventID))
		return 0
	}

	var wg sync.WaitGroup
	var successCount int
	var mu sync.Mutex

	for _, peerURL := range peers {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			if s.sendToPeer(ctx, url, event) {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(peerURL)
	}

	wg.Wait()

	s.log.Info("Broadcast complete",
		logger.String("event_id", event.EventID),
		logger.Int("total_peers", len(peers)),
		logger.Int("successful", successCount))

	return successCount
}

// Reset clears the sent cache (for testing or manual recovery).
func (s *CrossSyncService) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = make(map[string]bool)
}

func (s *CrossSyncService) sendToPeer(ctx context.Context, peerURL string, event *model.CrossRegionSyncEvent) bool {
	targetURL := peerURL + "/sync/cross-sync"

	body, err := json.Marshal(event)
	if err != nil {
		s.log.Error("Failed to marshal event for broadcast",
			logger.String("event_id", event.EventID),
			logger.Error(err))
		return false
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		s.log.Error("Failed to create broadcast request",
			logger.String("peer", peerURL),
			logger.String("event_id", event.EventID),
			logger.Error(err))
		s.pm.MarkUnhealthy(peerURL)
		return false
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		s.log.Warn("Broadcast to peer failed",
			logger.String("peer", peerURL),
			logger.String("event_id", event.EventID),
			logger.Error(err))
		s.pm.MarkUnhealthy(peerURL)
		return false
	}
	defer resp.Body.Close()

	// Drain body to free connection
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		s.pm.MarkHealthy(peerURL)
		s.log.Debug("Broadcast to peer succeeded",
			logger.String("peer", peerURL),
			logger.String("event_id", event.EventID),
			logger.Int("status", resp.StatusCode))
		return true
	}

	s.log.Warn("Broadcast to peer returned error status",
		logger.String("peer", peerURL),
		logger.String("event_id", event.EventID),
		logger.Int("status", resp.StatusCode))

	if resp.StatusCode >= 500 {
		s.pm.MarkUnhealthy(peerURL)
	}

	return false
}
