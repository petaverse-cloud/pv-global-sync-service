package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/service"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

// UserIndexReconciler periodically compares local user index with peer
// and syncs missing entries.
type UserIndexReconciler struct {
	indexSvc *service.GlobalIndexService
	peerURL  string
	httpCli  *http.Client
	log      *logger.Logger
	interval time.Duration
}

// NewUserIndexReconciler creates a new reconciler. Returns nil if peerURL is empty.
func NewUserIndexReconciler(indexSvc *service.GlobalIndexService, peerURL string, log *logger.Logger, interval time.Duration) *UserIndexReconciler {
	if peerURL == "" {
		return nil
	}
	return &UserIndexReconciler{
		indexSvc: indexSvc,
		peerURL:  peerURL,
		httpCli:  &http.Client{Timeout: 10 * time.Second},
		log:      log,
		interval: interval,
	}
}

// Run starts the reconciliation loop. Blocks until ctx is cancelled.
func (r *UserIndexReconciler) Run(ctx context.Context) {
	r.log.Info("User index reconciler started",
		logger.String("peer", r.peerURL),
		logger.String("interval", r.interval.String()))

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	// Run once immediately
	r.reconcile(ctx)

	for {
		select {
		case <-ctx.Done():
			r.log.Info("User index reconciler stopped")
			return
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

func (r *UserIndexReconciler) reconcile(ctx context.Context) {
	// 1. Fetch all user index entries from peer
	peerEntries, err := r.fetchPeerEntries(ctx)
	if err != nil {
		r.log.Warn("Reconciliation: failed to fetch peer entries",
			logger.String("peer", r.peerURL),
			logger.Error(err))
		return
	}

	r.log.Info("Reconciliation: fetched peer entries",
		logger.String("peer", r.peerURL),
		logger.Int("count", len(peerEntries)))

	// 2. Get local entries
	localEntries, err := r.indexSvc.GetAllUserIndexEntries(ctx)
	if err != nil {
		r.log.Warn("Reconciliation: failed to fetch local entries",
			logger.Error(err))
		return
	}

	// 3. Build local set
	localSet := make(map[string]string, len(localEntries))
	for _, e := range localEntries {
		localSet[e.EmailHash] = e.Region
	}

	// 4. Sync missing entries from peer
	synced := 0
	for _, e := range peerEntries {
		if _, exists := localSet[e.EmailHash]; !exists {
			// Entry missing locally, sync it
			if err := r.indexSvc.UpsertUserIndex(ctx, e.EmailHash, 0, e.Region, nil, "", ""); err != nil {
				r.log.Error("Reconciliation: failed to sync missing entry",
					logger.String("emailHash", e.EmailHash),
					logger.Error(err))
				continue
			}
			synced++
		}
	}

	if synced > 0 {
		r.log.Info("Reconciliation complete",
			logger.String("peer", r.peerURL),
			logger.Int("peerEntries", len(peerEntries)),
			logger.Int("localEntries", len(localEntries)),
			logger.Int("synced", synced))
	} else {
		r.log.Debug("Reconciliation complete: no missing entries",
			logger.String("peer", r.peerURL),
			logger.Int("peerEntries", len(peerEntries)),
			logger.Int("localEntries", len(localEntries)))
	}
}

func (r *UserIndexReconciler) fetchPeerEntries(ctx context.Context) ([]struct {
	EmailHash string
	Region    string
}, error) {
	url := r.peerURL + "/index/users/all"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := r.httpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result struct {
		Users []struct {
			EmailHash string `json:"emailHash"`
			Region    string `json:"region"`
		} `json:"users"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	entries := make([]struct {
		EmailHash string
		Region    string
	}, len(result.Users))
	for i, u := range result.Users {
		entries[i].EmailHash = u.EmailHash
		entries[i].Region = u.Region
	}
	return entries, nil
}
