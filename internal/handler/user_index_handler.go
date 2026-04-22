package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
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
	EmailHash string `json:"emailHash"`
	UserID    int64  `json:"userId"`
	Region    string `json:"region"`
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

	if req.EmailHash == "" || req.Region == "" {
		writeError(w, http.StatusBadRequest, "missing required fields")
		return
	}

	err := h.indexSvc.UpsertUserIndex(r.Context(), req.EmailHash, req.UserID, req.Region)
	if err != nil {
		h.log.Error("Failed to upsert user in global index", logger.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Fire-and-forget: broadcast to all healthy peers so they also have this user index
	go h.broadcastUserIndex(req)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// broadcastUserIndex sends the user index upsert to all healthy peers (fire-and-forget).
func (h *UserIndexHandler) broadcastUserIndex(req UpsertUserRequest) {
	body, err := json.Marshal(req)
	if err != nil {
		return
	}

	for _, peerURL := range h.pm.HealthyPeers() {
		url := peerURL + "/index/users/upsert"
		resp, err := h.httpCli.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			h.log.Debug("Failed to broadcast user index to peer", logger.String("peer", peerURL), logger.Error(err))
			continue
		}
		resp.Body.Close()
	}
}
