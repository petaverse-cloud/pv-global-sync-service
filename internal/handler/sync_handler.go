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

// GDPRCheckerIF defines the GDPR check interface for testability.
type GDPRCheckerIF interface {
	Check(event *model.CrossRegionSyncEvent) service.CheckResult
}

// AuditLoggerIF defines the audit log interface for testability.
type AuditLoggerIF interface {
	Log(ctx context.Context, event *model.CrossRegionSyncEvent, allowed bool, reason string) error
}

// FeedGenIF defines the feed generator interface for testability.
type FeedGenIF interface {
	HandleNewPost(ctx context.Context, authorUid int64, postUid int64) error
	HandleDeletedPost(ctx context.Context, postUid int64) error
}

// EventLogIF defines the event log interface for testability.
type EventLogIF interface {
	IsProcessed(ctx context.Context, eventID string) (bool, error)
	MarkProcessed(ctx context.Context, event *model.CrossRegionSyncEvent, errMsg string) error
}

// IndexSvcIF defines the global index service interface for testability.
type IndexSvcIF interface {
	InsertPost(ctx context.Context, event *model.CrossRegionSyncEvent) error
	UpdatePost(ctx context.Context, event *model.CrossRegionSyncEvent) error
	DeletePost(ctx context.Context, event *model.CrossRegionSyncEvent) error
	UpdateStats(ctx context.Context, postUid int64, likes, comments, shares, views int) error
	GetPost(ctx context.Context, postUid int64) (*model.GlobalPostIndex, error)
	GetPostByUid(ctx context.Context, postUid int64) (*model.GlobalPostIndex, error)
}

// TagIndexSvcIF defines the tag index service interface for testability.
type TagIndexSvcIF interface {
	UpsertTag(ctx context.Context, event *model.CrossRegionSyncEvent) error
	DeleteTag(ctx context.Context, event *model.CrossRegionSyncEvent) error
	UpdateStats(ctx context.Context, tagUID int64, postCount int64) error
	SearchTags(ctx context.Context, keyword string, limit int) ([]model.GlobalTagIndex, error)
	GetPopularTags(ctx context.Context, limit int) ([]model.GlobalTagIndex, error)
	GetTagByUID(ctx context.Context, tagUID int64) (*model.GlobalTagIndex, error)
	GetRegionsForTag(ctx context.Context, tagUID int64) ([]string, error)
}

// SyncHandler handles HTTP sync endpoints.
type SyncHandler struct {
	consumer      *consumer.SyncConsumer
	eventLog      EventLogIF
	gdprChecker   GDPRCheckerIF
	indexSvc      IndexSvcIF
	tagIndexSvc   TagIndexSvcIF
	auditSvc      AuditLoggerIF
	feedGenerator FeedGenIF
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
	tagIndexSvc *service.GlobalTagIndexService,
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
		tagIndexSvc:   tagIndexSvc,
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
		"status":   "accepted",
		"eventUid": event.EventID,
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
		"status":   "accepted",
		"eventUid": event.EventID,
	}); err != nil {
		h.log.Error("Failed to write response", logger.Error(err))
	}
}

// HandleGetPost handles GET /index/posts/:uid for querying the global index.
func (h *SyncHandler) HandleGetPost(w http.ResponseWriter, r *http.Request) {
	uidStr := chi.URLParam(r, "uid")
	if uidStr == "" {
		writeError(w, http.StatusBadRequest, "missing uid")
		return
	}

	uid, err := parseInt64(uidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid uid")
		return
	}

	post, err := h.indexSvc.GetPost(r.Context(), uid)
	if err != nil {
		h.log.Error("Failed to get post",
			logger.Int64("post_uid", uid),
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
		if err := h.feedGenerator.HandleNewPost(ctx, event.Payload.AuthorUid, event.Payload.PostUid); err != nil {
			h.log.Error("Feed generation failed, but post was synced",
				logger.Int64("post_uid", event.Payload.PostUid),
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
		if err := h.feedGenerator.HandleDeletedPost(ctx, event.Payload.PostUid); err != nil {
			h.log.Warn("Feed cache invalidation failed",
				logger.Int64("post_uid", event.Payload.PostUid),
				logger.Error(err))
		}
		return nil
	case model.EventTypePostStatsUpdated:
		return h.handleStatsUpdated(ctx, event)
	case model.EventTypeTagCreated, model.EventTypeTagUpdated:
		return h.tagIndexSvc.UpsertTag(ctx, event)
	case model.EventTypeTagDeleted:
		return h.tagIndexSvc.DeleteTag(ctx, event)
	case model.EventTypeTagStatsUpdated:
		if event.Payload.TagPostCount == nil {
			return nil // No post count to update — not an error
		}
		return h.tagIndexSvc.UpdateStats(ctx, event.Payload.TagUID, *event.Payload.TagPostCount)
	default:
		return nil
	}
}

// handleStatsUpdated reads actual stats from Regional DB and updates Global Index.
func (h *SyncHandler) handleStatsUpdated(ctx context.Context, event *model.CrossRegionSyncEvent) error {
	postUid := event.Payload.PostUid

	var likes, comments, favorites, views int
	// Note: Regional DB has favorites_count, not shares_count
	query := `SELECT likes_count, comments_count, favorites_count, views_count FROM posts WHERE uid = $1`
	err := h.regionalDB.QueryRow(ctx, query, postUid).Scan(&likes, &comments, &favorites, &views)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Post not in Regional DB (may have been synced from peer cluster).
			// Stats update is not actionable — skip silently.
			h.log.Debug("Post not found in Regional DB, skipping stats update",
				logger.Int64("post_uid", postUid))
			return nil
		}
		return fmt.Errorf("read stats for post uid=%d from regional db: %w", postUid, err)
	}

	if err := h.indexSvc.UpdateStats(ctx, postUid, likes, comments, favorites, views); err != nil {
		return fmt.Errorf("update stats for post uid=%d in global index: %w", postUid, err)
	}

	h.log.Info("Post stats updated in global index",
		logger.Int64("post_uid", postUid),
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

// HandleGetPostByUid handles GET /index/posts/uid/:uid for querying the global index by Snowflake ID.
func (h *SyncHandler) HandleGetPostByUid(w http.ResponseWriter, r *http.Request) {
	uidStr := chi.URLParam(r, "uid")
	if uidStr == "" {
		writeError(w, http.StatusBadRequest, "missing uid")
		return
	}

	uid, err := parseInt64(uidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid uid")
		return
	}

	post, err := h.indexSvc.GetPostByUid(r.Context(), uid)
	if err != nil {
		h.log.Error("Failed to get post by uid",
			logger.Int64("post_uid", uid),
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

// HandleSearchTags handles GET /index/tags/search?keyword=...&limit=...
func (h *SyncHandler) HandleSearchTags(w http.ResponseWriter, r *http.Request) {
	keyword := r.URL.Query().Get("keyword")
	limit := parseIntParam(r, "limit", 20)

	tags, err := h.tagIndexSvc.SearchTags(r.Context(), keyword, limit)
	if err != nil {
		h.log.Error("Search tags failed", logger.Error(err))
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"items": tags})
}

// HandlePopularTags handles GET /index/tags/popular?limit=...
func (h *SyncHandler) HandlePopularTags(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 20)

	tags, err := h.tagIndexSvc.GetPopularTags(r.Context(), limit)
	if err != nil {
		h.log.Error("Get popular tags failed", logger.Error(err))
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"items": tags})
}

// HandleGetTag handles GET /index/tags/{tagUid}
func (h *SyncHandler) HandleGetTag(w http.ResponseWriter, r *http.Request) {
	tagUIDStr := chi.URLParam(r, "tagUid")
	tagUID, err := parseInt64(tagUIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tagUid")
		return
	}

	tag, err := h.tagIndexSvc.GetTagByUID(r.Context(), tagUID)
	if err != nil {
		h.log.Error("Get tag failed", logger.Error(err))
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if tag == nil {
		writeError(w, http.StatusNotFound, "tag not found")
		return
	}

	writeJSON(w, http.StatusOK, tag)
}

// HandleGetTagRegions handles GET /index/tags/{tagUid}/regions
func (h *SyncHandler) HandleGetTagRegions(w http.ResponseWriter, r *http.Request) {
	tagUIDStr := chi.URLParam(r, "tagUid")
	tagUID, err := parseInt64(tagUIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tagUid")
		return
	}

	regions, err := h.tagIndexSvc.GetRegionsForTag(r.Context(), tagUID)
	if err != nil {
		h.log.Error("Get tag regions failed", logger.Error(err))
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"regions": regions})
}

func parseIntParam(r *http.Request, name string, defaultVal int) int {
	s := r.URL.Query().Get(name)
	if s == "" {
		return defaultVal
	}
	n, err := parseInt64(s)
	if err != nil || n < 1 {
		return defaultVal
	}
	return int(n)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
