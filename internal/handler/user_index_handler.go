package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/peer"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/service"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

type UserIndexHandler struct {
	indexSvc *service.GlobalIndexService
	pm       *peer.PeerManager
	log      *logger.Logger
	httpCli  *http.Client
}

func NewUserIndexHandler(indexSvc *service.GlobalIndexService, pm *peer.PeerManager, log *logger.Logger) *UserIndexHandler {
	return &UserIndexHandler{
		indexSvc: indexSvc,
		pm:       pm,
		log:      log,
		httpCli:  &http.Client{Timeout: 3 * time.Second},
	}
}

type CheckUserRequest struct {
	EmailHash string `json:"emailHash"`
}

type CheckUserResponse struct {
	Exists bool   `json:"exists"`
	Region string `json:"region,omitempty"`
}

type UpsertUserRequest struct {
	UID       int64   `json:"uid"`
	EmailHash *string `json:"emailHash,omitempty"`
	Region    string  `json:"region"`
}

// HandleCheckUser handles POST /index/users/check
func (h *UserIndexHandler) HandleCheckUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req CheckUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.EmailHash == "" {
		writeError(w, http.StatusBadRequest, "missing emailHash")
		return
	}

	region, err := h.indexSvc.FindRegionByEmailHash(r.Context(), req.EmailHash)
	if err != nil {
		h.log.Error("Failed to check user in global index", logger.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := CheckUserResponse{
		Exists: region != "",
		Region: region,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleUpsertUser handles POST /index/users/upsert
func (h *UserIndexHandler) HandleUpsertUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req UpsertUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.UID == 0 || req.Region == "" {
		writeError(w, http.StatusBadRequest, "missing required fields (uid, region)")
		return
	}

	err := h.indexSvc.UpsertUserIndex(r.Context(), req.UID, req.Region, req.EmailHash)
	if err != nil {
		h.log.Error("Failed to upsert user in global index", logger.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Fire-and-forget: broadcast to all healthy peers with retry
	go h.broadcastUserIndex(req)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// broadcastUserIndex sends the user index upsert to all healthy peers with retry.
func (h *UserIndexHandler) broadcastUserIndex(req UpsertUserRequest) {
	body, err := json.Marshal(req)
	if err != nil {
		h.log.Error("Failed to marshal user index for broadcast",
			logger.Int64("uid", req.UID),
			logger.Error(err))
		return
	}

	for _, peerURL := range h.pm.HealthyPeers() {
		url := peerURL + "/index/users/upsert"
		emailHashStr := ""
		if req.EmailHash != nil {
			emailHashStr = *req.EmailHash
		}
		success := h.sendWithRetry(url, body, emailHashStr)
		if !success {
			h.log.Warn("User index broadcast failed after retries",
				logger.String("peer", peerURL),
				logger.Int64("uid", req.UID))
		}
	}
}

// sendWithRetry POSTs the body to url with exponential backoff. Returns true on success.
func (h *UserIndexHandler) sendWithRetry(url string, body []byte, emailHash string) bool {
	const maxRetries = 3
	const baseDelay = 500 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			h.log.Error("Failed to create broadcast request",
				logger.String("url", url),
				logger.Error(err))
			return false
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := h.httpCli.Do(httpReq)
		if err != nil {
			if attempt < maxRetries-1 {
				delay := baseDelay << uint(attempt) // 500ms, 1s, 2s
				h.log.Debug("Broadcast failed, retrying",
					logger.String("url", url),
					logger.String("emailHash", emailHash),
					logger.Int("attempt", attempt+1),
					logger.String("delay", delay.String()),
					logger.Error(err))
				time.Sleep(delay)
				continue
			}
			h.log.Warn("Broadcast failed after all retries",
				logger.String("url", url),
				logger.String("emailHash", emailHash),
				logger.Error(err))
			return false
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return true
		}
		// Non-retryable server error
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			h.log.Warn("Broadcast rejected by peer (client error)",
				logger.String("url", url),
				logger.String("emailHash", emailHash),
				logger.Int("status", resp.StatusCode))
			return false
		}
		// 5xx: retry
		if attempt < maxRetries-1 {
			delay := baseDelay << uint(attempt)
			h.log.Debug("Broadcast returned 5xx, retrying",
				logger.String("url", url),
				logger.Int("status", resp.StatusCode),
				logger.Int("attempt", attempt+1))
			time.Sleep(delay)
		}
	}
	return false
}

// HandleGetUserRegion handles GET /index/user/region?uid=...
// Returns the region where the user with the given Snowflake uid is located.
func (h *UserIndexHandler) HandleGetUserRegion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	uidStr := r.URL.Query().Get("uid")
	if uidStr == "" {
		writeError(w, http.StatusBadRequest, "missing uid parameter")
		return
	}

	uid, err := strconv.ParseInt(uidStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid uid")
		return
	}

	region, err := h.indexSvc.FindRegionByUID(r.Context(), uid)
	if err != nil {
		h.log.Error("Failed to lookup user region",		logger.Int64("uid", uid),logger.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if region == "" {
		writeError(w, http.StatusNotFound, "user not found in global index")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"region": region})
}

// HandleGetAllUsers handles GET /index/users/all - returns all user index entries for reconciliation.
func (h *UserIndexHandler) HandleGetAllUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	entries, err := h.indexSvc.GetAllUserIndexEntries(r.Context())
	if err != nil {
		h.log.Error("Failed to get all user index entries", logger.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	type UserEntry struct {
		UID       int64   `json:"uid"`
		Region    string  `json:"region"`
		EmailHash *string `json:"emailHash,omitempty"`
	}
	users := make([]UserEntry, len(entries))
	for i, e := range entries {
		users[i].UID = e.UID
		users[i].Region = e.Region
		users[i].EmailHash = e.EmailHash
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"count": len(users),
		"users": users,
	})
}
