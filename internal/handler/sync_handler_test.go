package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/peer"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/sync"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

// TestSyncHandler_HandleCrossSync_InvalidJSON verifies input validation
// before processEvent is called (no nil deps needed).
func TestSyncHandler_HandleCrossSync_InvalidJSON(t *testing.T) {
	log, _ := logger.New("warn", "console")
	pm := peer.NewPeerManager([]string{}, 100*time.Millisecond)
	crossSync := sync.NewCrossSyncService(pm, 100*time.Millisecond, log)

	h := &SyncHandler{
		crossSync: crossSync,
		log:       log,
	}

	req := httptest.NewRequest(http.MethodPost, "/sync/cross-sync", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	h.HandleCrossSync(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleCrossSync() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestSyncHandler_HandleCrossSync_MethodNotAllowed verifies HTTP method check.
func TestSyncHandler_HandleCrossSync_MethodNotAllowed(t *testing.T) {
	log, _ := logger.New("warn", "console")
	pm := peer.NewPeerManager([]string{}, 100*time.Millisecond)
	crossSync := sync.NewCrossSyncService(pm, 100*time.Millisecond, log)

	h := &SyncHandler{
		crossSync: crossSync,
		log:       log,
	}

	req := httptest.NewRequest(http.MethodGet, "/sync/cross-sync", nil)
	rec := httptest.NewRecorder()

	h.HandleCrossSync(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("HandleCrossSync() status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// TestSyncHandler_HandleSync_InvalidJSON verifies input validation.
func TestSyncHandler_HandleSync_InvalidJSON(t *testing.T) {
	log, _ := logger.New("warn", "console")

	h := &SyncHandler{
		log: log,
	}

	req := httptest.NewRequest(http.MethodPost, "/sync/content", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	h.HandleSync(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleSync() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
