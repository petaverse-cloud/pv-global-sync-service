package handler

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/service"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

func makeEvent(eventType model.SyncEventType, postUid, authorUid int64, content string) *model.CrossRegionSyncEvent {
	return &model.CrossRegionSyncEvent{
		EventID: "evt_test_001", EventType: eventType,
		SourceRegion: model.RegionSEA, TargetRegion: model.RegionEU,
		Timestamp: time.Now().Unix(),
		Payload: model.EventPayload{
			PostUid: postUid, AuthorUid: authorUid,
			AuthorRegion: model.RegionSEA, Visibility: model.VisibilityGlobal,
			Content: content,
		},
		Metadata: model.EventMetadata{
			GDPRCompliant: true, UserConsent: true,
			DataCategory: model.DataCategoryUGC, CrossBorderOK: true,
		},
	}
}

// ===== Mocks =====

type mockIndexSvc struct {
	insertErr, updateErr, deleteErr, updateStatsErr error
}

func (m *mockIndexSvc) InsertPost(_ context.Context, _ *model.CrossRegionSyncEvent) error { return m.insertErr }
func (m *mockIndexSvc) UpdatePost(_ context.Context, _ *model.CrossRegionSyncEvent) error { return m.updateErr }
func (m *mockIndexSvc) DeletePost(_ context.Context, _ *model.CrossRegionSyncEvent) error { return m.deleteErr }
func (m *mockIndexSvc) UpdateStats(_ context.Context, _ int64, _, _, _, _ int) error    { return m.updateStatsErr }
func (m *mockIndexSvc) GetPost(_ context.Context, _ int64) (*model.GlobalPostIndex, error) { return nil, nil }
func (m *mockIndexSvc) GetPostByUid(_ context.Context, _ int64) (*model.GlobalPostIndex, error) { return nil, nil }

type mockFeedGen struct{ newPostCalled, deletedPostCalled bool }

func (m *mockFeedGen) HandleNewPost(_ context.Context, _, _ int64) error {
	m.newPostCalled = true; return nil
}
func (m *mockFeedGen) HandleDeletedPost(_ context.Context, _ int64) error {
	m.deletedPostCalled = true; return nil
}

type mockEventLog struct{ processed map[string]bool }

func (m *mockEventLog) IsProcessed(_ context.Context, id string) (bool, error) {
	return m.processed[id], nil
}
func (m *mockEventLog) MarkProcessed(_ context.Context, ev *model.CrossRegionSyncEvent, _ string) error {
	m.processed[ev.EventID] = true; return nil
}

type mockGDPR struct{ allowed bool }

func (m *mockGDPR) Check(_ *model.CrossRegionSyncEvent) service.CheckResult {
	return service.CheckResult{Allowed: m.allowed}
}

type mockAudit struct{}

func (m *mockAudit) Log(_ context.Context, _ *model.CrossRegionSyncEvent, _ bool, _ string) error { return nil }

type mockTagSvc struct{ upsert, del, updStats bool }

func (m *mockTagSvc) UpsertTag(_ context.Context, _ *model.CrossRegionSyncEvent) error    { m.upsert = true; return nil }
func (m *mockTagSvc) DeleteTag(_ context.Context, _ *model.CrossRegionSyncEvent) error    { m.del = true; return nil }
func (m *mockTagSvc) UpdateStats(_ context.Context, _, _ int64) error                     { m.updStats = true; return nil }
func (m *mockTagSvc) SearchTags(_ context.Context, _ string, _ int) ([]model.GlobalTagIndex, error) { return nil, nil }
func (m *mockTagSvc) GetPopularTags(_ context.Context, _ int) ([]model.GlobalTagIndex, error)       { return nil, nil }
func (m *mockTagSvc) GetTagByUID(_ context.Context, _ int64) (*model.GlobalTagIndex, error)         { return nil, nil }
func (m *mockTagSvc) GetRegionsForTag(_ context.Context, _ int64) ([]string, error)                 { return nil, nil }

// ===== routeEvent tests =====

func TestRouteEvent_PostCreated(t *testing.T) {
	feed := &mockFeedGen{}
	h := &SyncHandler{indexSvc: &mockIndexSvc{}, feedGenerator: feed, log: logger.NewNop()}
	err := h.routeEvent(context.Background(), makeEvent(model.EventTypePostCreated, 100, 200, "hi"))
	if err != nil { t.Fatal(err) }
	if !feed.newPostCalled { t.Error("HandleNewPost not called") }
}

func TestRouteEvent_PostUpdated(t *testing.T) {
	h := &SyncHandler{indexSvc: &mockIndexSvc{}, log: logger.NewNop()}
	if err := h.routeEvent(context.Background(), makeEvent(model.EventTypePostUpdated, 101, 201, "")); err != nil {
		t.Fatal(err)
	}
}

func TestRouteEvent_PostDeleted(t *testing.T) {
	feed := &mockFeedGen{}
	h := &SyncHandler{indexSvc: &mockIndexSvc{}, feedGenerator: feed, log: logger.NewNop()}
	if err := h.routeEvent(context.Background(), makeEvent(model.EventTypePostDeleted, 102, 202, "")); err != nil {
		t.Fatal(err)
	}
	if !feed.deletedPostCalled { t.Error("HandleDeletedPost not called") }
}

func TestRouteEvent_TagCreated(t *testing.T) {
	tag := &mockTagSvc{}
	h := &SyncHandler{tagIndexSvc: tag, log: logger.NewNop()}
	ev := makeEvent(model.EventTypeTagCreated, 0, 0, ""); ev.Payload.TagUID = 500
	if err := h.routeEvent(context.Background(), ev); err != nil { t.Fatal(err) }
	if !tag.upsert { t.Error("UpsertTag not called") }
}

func TestRouteEvent_TagUpdated(t *testing.T) {
	tag := &mockTagSvc{}
	h := &SyncHandler{tagIndexSvc: tag, log: logger.NewNop()}
	ev := makeEvent(model.EventTypeTagUpdated, 0, 0, ""); ev.Payload.TagUID = 501
	if err := h.routeEvent(context.Background(), ev); err != nil { t.Fatal(err) }
	if !tag.upsert { t.Error("UpsertTag not called") }
}

func TestRouteEvent_TagDeleted(t *testing.T) {
	tag := &mockTagSvc{}
	h := &SyncHandler{tagIndexSvc: tag, log: logger.NewNop()}
	ev := makeEvent(model.EventTypeTagDeleted, 0, 0, ""); ev.Payload.TagUID = 502
	if err := h.routeEvent(context.Background(), ev); err != nil { t.Fatal(err) }
	if !tag.del { t.Error("DeleteTag not called") }
}

func TestRouteEvent_TagStatsUpdated(t *testing.T) {
	tag := &mockTagSvc{}
	h := &SyncHandler{tagIndexSvc: tag, log: logger.NewNop()}
	ev := makeEvent(model.EventTypeTagStatsUpdated, 0, 0, "")
	ev.Payload.TagUID = 503; pc := int64(42); ev.Payload.TagPostCount = &pc
	if err := h.routeEvent(context.Background(), ev); err != nil { t.Fatal(err) }
	if !tag.updStats { t.Error("UpdateStats not called") }
}

func TestRouteEvent_UnknownType(t *testing.T) {
	h := &SyncHandler{log: logger.NewNop()}
	if err := h.routeEvent(context.Background(), makeEvent("UNKNOWN", 0, 0, "")); err != nil {
		t.Fatal(err)
	}
}

func TestRouteEvent_InsertError(t *testing.T) {
	h := &SyncHandler{indexSvc: &mockIndexSvc{insertErr: errors.New("db down")}, log: logger.NewNop()}
	if err := h.routeEvent(context.Background(), makeEvent(model.EventTypePostCreated, 100, 200, "")); err == nil {
		t.Fatal("expected error")
	}
}

// ===== processEvent tests =====

func TestProcessEvent_Success(t *testing.T) {
	el := &mockEventLog{processed: map[string]bool{}}
	h := &SyncHandler{
		eventLog: el, gdprChecker: &mockGDPR{allowed: true}, auditSvc: &mockAudit{},
		indexSvc: &mockIndexSvc{}, feedGenerator: &mockFeedGen{}, log: logger.NewNop(),
	}
	if err := h.processEvent(context.Background(), makeEvent(model.EventTypePostCreated, 900, 800, "hi"), "local_api"); err != nil {
		t.Fatal(err)
	}
	if !el.processed["evt_test_001"] { t.Error("not marked processed") }
}

func TestProcessEvent_Idempotent(t *testing.T) {
	el := &mockEventLog{processed: map[string]bool{"evt_test_001": true}}
	h := &SyncHandler{eventLog: el, log: logger.NewNop()}
	if err := h.processEvent(context.Background(), makeEvent(model.EventTypePostCreated, 900, 800, "hi"), "x"); err != nil {
		t.Fatal(err)
	}
}

func TestProcessEvent_GDPRDenied(t *testing.T) {
	el := &mockEventLog{processed: map[string]bool{}}
	h := &SyncHandler{eventLog: el, gdprChecker: &mockGDPR{allowed: false}, auditSvc: &mockAudit{}, log: logger.NewNop()}
	if err := h.processEvent(context.Background(), makeEvent(model.EventTypePostCreated, 900, 800, "hi"), "x"); err != nil {
		t.Fatal(err)
	}
	if !el.processed["evt_test_001"] { t.Error("not marked processed for denied event") }
}

func TestProcessEvent_RouteError(t *testing.T) {
	el := &mockEventLog{processed: map[string]bool{}}
	h := &SyncHandler{
		eventLog: el, gdprChecker: &mockGDPR{allowed: true}, auditSvc: &mockAudit{},
		indexSvc: &mockIndexSvc{insertErr: errors.New("db down")}, log: logger.NewNop(),
	}
	if err := h.processEvent(context.Background(), makeEvent(model.EventTypePostCreated, 900, 800, "hi"), "x"); err == nil {
		t.Fatal("expected error")
	}
	if !el.processed["evt_test_001"] { t.Error("not marked processed for failed event") }
}

func TestProcessEvent_MissingFields(t *testing.T) {
	h := &SyncHandler{log: logger.NewNop()}
	if err := h.processEvent(context.Background(), &model.CrossRegionSyncEvent{}, "x"); err == nil {
		t.Fatal("expected error")
	}
}

// ===== HandleGetPost tests =====

func TestHandleGetPost_Found(t *testing.T) {
	mock, _ := pgxmock.NewPool(); defer mock.Close()
	now := time.Now()
	rows := pgxmock.NewRows([]string{
		"post_slug", "author_uid", "author_region", "content_preview", "visibility",
		"hashtags", "mentions", "media_urls_str",
		"likes_count", "comments_count", "shares_count", "views_count",
		"gdpr_compliant", "user_consent", "data_category", "created_at", "synced_at",
		"author_nickname", "author_avatar_url",
	}).AddRow(int64(12345), int64(67890), "SEA", "Hello", "GLOBAL",
		nil, nil, "", 0, 0, 0, 0, true, true, "TIER_2", now, now, nil, nil)
	mock.ExpectQuery("SELECT").WithArgs(int64(12345)).WillReturnRows(rows)

	h := &SyncHandler{indexSvc: service.NewGlobalIndexServiceWithDB(mock, logger.NewNop()), log: logger.NewNop()}
	r := chi.NewRouter(); r.Get("/index/posts/{uid}", h.HandleGetPost)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/index/posts/12345", nil))
	if rec.Code != 200 { t.Errorf("status=%d", rec.Code) }
}

func TestHandleGetPost_NotFound(t *testing.T) {
	mock, _ := pgxmock.NewPool(); defer mock.Close()
	mock.ExpectQuery("SELECT").WithArgs(int64(99999)).WillReturnError(pgx.ErrNoRows)
	h := &SyncHandler{indexSvc: service.NewGlobalIndexServiceWithDB(mock, logger.NewNop()), log: logger.NewNop()}
	r := chi.NewRouter(); r.Get("/index/posts/{uid}", h.HandleGetPost)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/index/posts/99999", nil))
	if rec.Code != 404 { t.Errorf("status=%d", rec.Code) }
}

func TestHandleGetPost_InvalidUid(t *testing.T) {
	h := &SyncHandler{log: logger.NewNop()}
	r := chi.NewRouter(); r.Get("/index/posts/{uid}", h.HandleGetPost)
	for _, uid := range []string{"abc", "-5", "12a34"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", "/index/posts/"+uid, nil))
		if rec.Code != 400 { t.Errorf("uid=%q status=%d", uid, rec.Code) }
	}
}

// ===== HandleSync validation tests =====

func TestHandleSync_InvalidJSON(t *testing.T) {
	h := &SyncHandler{log: logger.NewNop()}
	r := chi.NewRouter(); r.Post("/sync/content", h.HandleSync)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/sync/content", strings.NewReader("bad")))
	if rec.Code != 400 { t.Errorf("status=%d", rec.Code) }
}

func TestHandleSync_MissingFields(t *testing.T) {
	h := &SyncHandler{log: logger.NewNop()}
	r := chi.NewRouter(); r.Post("/sync/content", h.HandleSync)
	for _, tt := range []struct{ n, b string }{
		{"no eventId", `{"eventType":"POST_CREATED","payload":{"postUid":1,"authorUid":2}}`},
		{"no eventType", `{"eventId":"evt_1","payload":{"postUid":1,"authorUid":2}}`},
		{"empty", `{}`},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/sync/content", strings.NewReader(tt.b))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(rec, req)
		if rec.Code != 400 { t.Errorf("%s: status=%d", tt.n, rec.Code) }
	}
}
