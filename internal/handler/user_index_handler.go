package handler

import (
	"encoding/json"
	"net/http"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/service"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

type UserIndexHandler struct {
	indexSvc *service.GlobalIndexService
	log      *logger.Logger
}

func NewUserIndexHandler(indexSvc *service.GlobalIndexService, log *logger.Logger) *UserIndexHandler {
	return &UserIndexHandler{indexSvc: indexSvc, log: log}
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
