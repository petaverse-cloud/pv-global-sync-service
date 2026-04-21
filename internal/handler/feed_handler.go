// Package handler implements HTTP request handlers for the Global Sync Service.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/service"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

// FeedGenerator defines the interface for feed generation.
type FeedGenerator interface {
	GetFeed(ctx context.Context, userID int64, feedType string, cursor string, limit int) ([]service.FeedItem, string, bool, error)
}

// FeedHandler handles feed API endpoints.
type FeedHandler struct {
	generator FeedGenerator
	log       *logger.Logger
}

// NewFeedHandler creates a new feed handler.
func NewFeedHandler(generator FeedGenerator, log *logger.Logger) *FeedHandler {
	return &FeedHandler{generator: generator, log: log}
}

// HandleGetFeed handles GET /feed/:userId
//
// Query parameters:
//   - feedType: following | global | trending (default: following)
//   - limit: number of items (default: 20, max: 100)
//   - cursor: pagination cursor (optional)
func (h *FeedHandler) HandleGetFeed(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "userId")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid userId")
		return
	}

	feedType := r.URL.Query().Get("feedType")
	if feedType == "" {
		feedType = "following"
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, parseErr := strconv.Atoi(l); parseErr == nil && n > 0 {
			limit = n
			if limit > 100 {
				limit = 100
			}
		}
	}

	cursor := r.URL.Query().Get("cursor")

	items, nextCursor, hasMore, err := h.generator.GetFeed(r.Context(), userID, feedType, cursor, limit)
	if err != nil {
		h.log.Error("Failed to generate feed",
			logger.Int64("user_id", userID),
			logger.String("feed_type", feedType),
			logger.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to generate feed")
		return
	}

	response := map[string]interface{}{
		"items":      items,
		"nextCursor": nextCursor,
		"hasMore":    hasMore,
		"metadata": map[string]interface{}{
			"feedType": feedType,
			"limit":    limit,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.log.Error("Failed to encode response", logger.Error(err))
	}
}
