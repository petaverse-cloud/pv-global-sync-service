// Package handler implements HTTP request handlers for the Global Sync Service.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/consumer"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/service"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/sync"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

// SyncHandler handles HTTP sync endpoints.
type SyncHandler struct {
	consumer      *consumer.SyncConsumer
	eventLog      *service.SyncEventLogService
	gdprChecker   *service.GDPRChecker
	indexSvc      *service.GlobalIndexService
	auditSvc      *service.AuditLogService
	feedGenerator *service.FeedGenerator
	regionalDB    *pgxpool.Pool
	crossSync     *sync.CrossSyncService
	log           *logger.Logger
}

// NewSyncHandler creates a new sync handler.
func NewSyncHandler(
	consumer *consumer.SyncConsumer,
	eventLog *service.SyncEventLogService,
	gdprChecker *service.GDPRChecker,
	indexSvc *service.GlobalIndexService,
	auditSvc *service.AuditLogService,
	feedGenerator *service.FeedGenerator,
	regionalDB *pgxpool.Pool,
	crossSync *sync.CrossSyncService,
	log *logger.Logger,
) *SyncHandler {
	return &SyncHandler{
		consumer:      consumer,
		eventLog:      eventLog,
		gdprChecker:   gdprChecker,
		indexSvc:      indexSvc,
		auditSvc:      auditSvc,
		feedGenerator: feedGenerator,
		regionalDB:    regionalDB,
		crossSync:     crossSync,
		log:           log,
	}
}

// HandleSync handles POST /sync/content from the local WigoWago API.
func (h *SyncHandler) HandleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var event model.CrossRegionSyncEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Validate required fields
	if event.EventID == "" || event.EventType == "" {
		writeError(w, http.StatusBadRequest, "missing required fields: eventId and eventType are required")
		return
	}

	if err := h.processEvent(r.Context(), &event, "local_api"); err != nil {
		h.log.Error("Sync handler failed",
			logger.String("event_id", event.EventID),
			logger.Error(err))
		writeError(w, http.StatusInternalServerError, "processing failed: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":  "accepted",
		"eventId": event.EventID,
	}); err != nil {
		h.log.Error("Failed to write response", logger.Error(err))
	}
}

// HandleCrossSync handles POST /sync/cross-sync from the peer region's sync service.
func (h *SyncHandler) HandleCrossSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var event model.CrossRegionSyncEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Validate required fields
	if event.EventID == "" || event.EventType == "" {
		writeError(w, http.StatusBadRequest, "missing required fields: eventId and eventType are required")
		return
	}

	if err := h.processEvent(r.Context(), &event, "cross_sync"); err != nil {
		h.log.Error("Cross-sync handler failed",
			logger.String("event_id", event.EventID),
			logger.Error(err))
		writeError(w, http.StatusInternalServerError, "processing failed: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":  "accepted",
		"eventId": event.EventID,
	}); err != nil {
		h.log.Error("Failed to write response", logger.Error(err))
	}
}

// HandleGetPost handles GET /index/posts/:postId for querying the global index.
func (h *SyncHandler) HandleGetPost(w http.ResponseWriter, r *http.Request) {
	postIDStr := chi.URLParam(r, "postId")
	if postIDStr == "" {
		writeError(w, http.StatusBadRequest, "missing postId")
		return
	}

	postID, err := parseInt64(postIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid postId")
		return
	}

	post, err := h.indexSvc.GetPost(r.Context(), postID)
	if err != nil {
		h.log.Error("Failed to get post",
			logger.Int64("post_id", postID),
			logger.Error(err))
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	if post == nil {
		writeError(w, http.StatusNotFound, "post not found in global index")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(post); err != nil {
		h.log.Error("Failed to encode post", logger.Error(err))
	}
}

func parseInt64(s string) (int64, error) {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errNotDigit
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

var errNotDigit = fmt.Errorf("not a digit")

// processEvent runs the full sync pipeline for an event.
func (h *SyncHandler) processEvent(ctx context.Context, event *model.CrossRegionSyncEvent, source string) error {
	// Validate
	if event.EventID == "" || event.EventType == "" {
		return fmt.Errorf("missing required fields: eventId and eventType are required")
	}

	// Idempotency
	processed, err := h.eventLog.IsProcessed(ctx, event.EventID)
	if err != nil {
		return err
	}
	if processed {
		h.log.Debug("Event already processed",
			logger.String("event_id", event.EventID),
			logger.String("source", source))
		return nil
	}

	// GDPR check
	result := h.gdprChecker.Check(event)
	h.auditSvc.Log(ctx, event, result.Allowed, result.Reason) //nolint:errcheck

	if !result.Allowed {
		h.eventLog.MarkProcessed(ctx, event, "gdpr_denied: "+result.Reason) //nolint:errcheck
		return nil                                                          // Denied is not an error from the caller's perspective
	}

	// Route to handler
	if err := h.routeEvent(ctx, event); err != nil {
		h.eventLog.MarkProcessed(ctx, event, err.Error()) //nolint:errcheck
		return err
	}

	h.eventLog.MarkProcessed(ctx, event, "") //nolint:errcheck

	// Broadcast to peer clusters (fire-and-forget, only for local API events)
	if source == "local_api" && h.crossSync != nil {
		go func() {
			bctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			h.crossSync.Broadcast(bctx, event)
		}()
	}

	return nil
}

// routeEvent dispatches to the appropriate index operation.
func (h *SyncHandler) routeEvent(ctx context.Context, event *model.CrossRegionSyncEvent) error {
	switch event.EventType {
	case model.EventTypePostCreated:
		if err := h.indexSvc.InsertPost(ctx, event); err != nil {
			return err
		}
		// Trigger feed generation after successful insert
		if err := h.feedGenerator.HandleNewPost(ctx, event.Payload.AuthorID, event.Payload.PostID); err != nil {
			h.log.Error("Feed generation failed, but post was synced",
				logger.Int64("post_id", event.Payload.PostID),
				logger.Error(err))
		}
		return nil
	case model.EventTypePostUpdated:
		return h.indexSvc.UpdatePost(ctx, event)
	case model.EventTypePostDeleted:
		if err := h.indexSvc.DeletePost(ctx, event); err != nil {
			return err
		}
		// Invalidate feed caches
		if err := h.feedGenerator.HandleDeletedPost(ctx, event.Payload.PostID); err != nil {
			h.log.Warn("Feed cache invalidation failed",
				logger.Int64("post_id", event.Payload.PostID),
				logger.Error(err))
		}
		return nil
	case model.EventTypePostStatsUpdated:
		return h.handleStatsUpdated(ctx, event)
	default:
		return nil
	}
}

// handleStatsUpdated reads actual stats from Regional DB and updates Global Index.
func (h *SyncHandler) handleStatsUpdated(ctx context.Context, event *model.CrossRegionSyncEvent) error {
	postSlug := event.Payload.PostSlug
	postID := event.Payload.PostID

	var likes, comments, favorites, views int
	// Note: Regional DB has favorites_count, not shares_count
	query := `SELECT likes_count, comments_count, favorites_count, views_count FROM posts WHERE post_id = $1`
	err := h.regionalDB.QueryRow(ctx, query, postID).Scan(&likes, &comments, &favorites, &views)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Post not in Regional DB (may have been synced from peer cluster).
			// Stats update is not actionable — skip silently.
			h.log.Debug("Post not found in Regional DB, skipping stats update",
				logger.Int64("post_id", postID))
			return nil
		}
		return fmt.Errorf("read stats for post %d from regional db: %w", postID, err)
	}

	if err := h.indexSvc.UpdateStats(ctx, postSlug, likes, comments, favorites, views); err != nil {
		return fmt.Errorf("update stats for post slug=%d in global index: %w", postSlug, err)
	}

	h.log.Info("Post stats updated in global index",
		logger.Int64("post_id", postID),
		logger.Int64("post_slug", postSlug),
		logger.Int("likes", likes),
		logger.Int("comments", comments),
		logger.Int("favorites", favorites),
		logger.Int("views", views))

	return nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   http.StatusText(status),
		"message": message,
	})
}

// HandleGetPostBySlug handles GET /index/posts/slug/:slug for querying the global index by Snowflake ID.
func (h *SyncHandler) HandleGetPostBySlug(w http.ResponseWriter, r *http.Request) {
	slugStr := chi.URLParam(r, "slug")
	if slugStr == "" {
		writeError(w, http.StatusBadRequest, "missing slug")
		return
	}

	slug, err := parseInt64(slugStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid slug")
		return
	}

	post, err := h.indexSvc.GetPostBySlug(r.Context(), slug)
	if err != nil {
		h.log.Error("Failed to get post by slug",
			logger.Int64("post_slug", slug),
			logger.Error(err))
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	if post == nil {
		writeError(w, http.StatusNotFound, "post not found in global index")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(post); err != nil {
		h.log.Error("Failed to encode post", logger.Error(err))
	}
}
