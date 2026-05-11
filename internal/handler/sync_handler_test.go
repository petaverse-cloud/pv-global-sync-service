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
		Payload: model.EventPayload{PostUid: postUid, AuthorUid: authorUid, AuthorRegion: model.RegionSEA, Visibility: model.VisibilityGlobal, Content: content},
		Metadata: model.EventMetadata{GDPRCompliant: true, UserConsent: true, DataCategory: model.DataCategoryUGC, CrossBorderOK: true},
	}
}

type mockIdx struct{ insErr, updErr, delErr, statErr error }
func (m *mockIdx) InsertPost(_ context.Context, _ *model.CrossRegionSyncEvent) error { return m.insErr }
func (m *mockIdx) UpdatePost(_ context.Context, _ *model.CrossRegionSyncEvent) error { return m.updErr }
func (m *mockIdx) DeletePost(_ context.Context, _ *model.CrossRegionSyncEvent) error { return m.delErr }
func (m *mockIdx) UpdateStats(_ context.Context, _ int64, _, _, _, _ int) error        { return m.statErr }
func (m *mockIdx) GetPost(_ context.Context, _ int64) (*model.GlobalPostIndex, error)    { return nil, nil }
func (m *mockIdx) GetPostByUid(_ context.Context, _ int64) (*model.GlobalPostIndex, error) { return nil, nil }

type mockFeed struct{ newPost, delPost bool }
func (m *mockFeed) HandleNewPost(_ context.Context, _, _ int64) error { m.newPost = true; return nil }
func (m *mockFeed) HandleDeletedPost(_ context.Context, _ int64) error { m.delPost = true; return nil }

type mockEvt struct{ m map[string]bool }
func (m *mockEvt) IsProcessed(_ context.Context, id string) (bool, error) { return m.m[id], nil }
func (m *mockEvt) MarkProcessed(_ context.Context, ev *model.CrossRegionSyncEvent, _ string) error { m.m[ev.EventID] = true; return nil }

type mockGDPR struct{ a bool }
func (m *mockGDPR) Check(_ *model.CrossRegionSyncEvent) service.CheckResult { return service.CheckResult{Allowed: m.a} }
type mockAudit struct{}
func (m *mockAudit) Log(_ context.Context, _ *model.CrossRegionSyncEvent, _ bool, _ string) error { return nil }

type mockTag struct{ upsert, del, updStats bool; tag *model.GlobalTagIndex; regions []string }
func (m *mockTag) UpsertTag(_ context.Context, _ *model.CrossRegionSyncEvent) error    { m.upsert = true; return nil }
func (m *mockTag) DeleteTag(_ context.Context, _ *model.CrossRegionSyncEvent) error    { m.del = true; return nil }
func (m *mockTag) UpdateStats(_ context.Context, _, _ int64) error                     { m.updStats = true; return nil }
func (m *mockTag) SearchTags(_ context.Context, _ string, _ int) ([]model.GlobalTagIndex, error) { return nil, nil }
func (m *mockTag) GetPopularTags(_ context.Context, _ int) ([]model.GlobalTagIndex, error)       { return nil, nil }
func (m *mockTag) GetTagByUID(_ context.Context, _ int64) (*model.GlobalTagIndex, error)         { return m.tag, nil }
func (m *mockTag) GetRegionsForTag(_ context.Context, _ int64) ([]string, error)                 { return m.regions, nil }

// ===== routeEvent =====

func TestRouteEvent_PostCreated(t *testing.T) {
	f := &mockFeed{}
	h := &SyncHandler{indexSvc: &mockIdx{}, feedGenerator: f, log: logger.NewNop()}
	if err := h.routeEvent(context.Background(), makeEvent(model.EventTypePostCreated, 100, 200, "hi")); err != nil { t.Fatal(err) }
	if !f.newPost { t.Error("HandleNewPost not called") }
}
func TestRouteEvent_PostUpdated(t *testing.T) {
	h := &SyncHandler{indexSvc: &mockIdx{}, log: logger.NewNop()}
	if err := h.routeEvent(context.Background(), makeEvent(model.EventTypePostUpdated, 101, 201, "")); err != nil { t.Fatal(err) }
}
func TestRouteEvent_PostDeleted(t *testing.T) {
	f := &mockFeed{}
	h := &SyncHandler{indexSvc: &mockIdx{}, feedGenerator: f, log: logger.NewNop()}
	if err := h.routeEvent(context.Background(), makeEvent(model.EventTypePostDeleted, 102, 202, "")); err != nil { t.Fatal(err) }
	if !f.delPost { t.Error("HandleDeletedPost not called") }
}
func TestRouteEvent_TagCreated(t *testing.T) {
	tg := &mockTag{}
	h := &SyncHandler{tagIndexSvc: tg, log: logger.NewNop()}
	ev := makeEvent(model.EventTypeTagCreated, 0, 0, ""); ev.Payload.TagUID = 500
	if err := h.routeEvent(context.Background(), ev); err != nil { t.Fatal(err) }
	if !tg.upsert { t.Error("UpsertTag not called") }
}
func TestRouteEvent_TagUpdated(t *testing.T) {
	tg := &mockTag{}
	h := &SyncHandler{tagIndexSvc: tg, log: logger.NewNop()}
	ev := makeEvent(model.EventTypeTagUpdated, 0, 0, ""); ev.Payload.TagUID = 501
	if err := h.routeEvent(context.Background(), ev); err != nil { t.Fatal(err) }
	if !tg.upsert { t.Error("UpsertTag not called") }
}
func TestRouteEvent_TagDeleted(t *testing.T) {
	tg := &mockTag{}
	h := &SyncHandler{tagIndexSvc: tg, log: logger.NewNop()}
	ev := makeEvent(model.EventTypeTagDeleted, 0, 0, ""); ev.Payload.TagUID = 502
	if err := h.routeEvent(context.Background(), ev); err != nil { t.Fatal(err) }
	if !tg.del { t.Error("DeleteTag not called") }
}
func TestRouteEvent_TagStatsUpdated(t *testing.T) {
	tg := &mockTag{}
	h := &SyncHandler{tagIndexSvc: tg, log: logger.NewNop()}
	ev := makeEvent(model.EventTypeTagStatsUpdated, 0, 0, "")
	ev.Payload.TagUID = 503; pc := int64(42); ev.Payload.TagPostCount = &pc
	if err := h.routeEvent(context.Background(), ev); err != nil { t.Fatal(err) }
	if !tg.updStats { t.Error("UpdateStats not called") }
}
func TestRouteEvent_UnknownType(t *testing.T) {
	h := &SyncHandler{log: logger.NewNop()}
	if err := h.routeEvent(context.Background(), makeEvent("UNKNOWN", 0, 0, "")); err != nil { t.Fatal(err) }
}
func TestRouteEvent_InsertError(t *testing.T) {
	h := &SyncHandler{indexSvc: &mockIdx{insErr: errors.New("db")}, log: logger.NewNop()}
	if err := h.routeEvent(context.Background(), makeEvent(model.EventTypePostCreated, 100, 200, "")); err == nil { t.Fatal("expected err") }
}

// ===== BUG: nil TagPostCount causes panic =====

func TestRouteEvent_TagStatsUpdated_NilPostCount_NoPanic(t *testing.T) {
	tg := &mockTag{}
	h := &SyncHandler{tagIndexSvc: tg, log: logger.NewNop()}
	ev := makeEvent(model.EventTypeTagStatsUpdated, 0, 0, "")
	ev.Payload.TagUID = 999
	ev.Payload.TagPostCount = nil

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PANIC on nil TagPostCount: %v", r)
		}
	}()
	err := h.routeEvent(context.Background(), ev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Log("Bug fixed: no panic on nil TagPostCount")
}

// ===== processEvent =====

func TestProcessEvent_Success(t *testing.T) {
	el := &mockEvt{m: map[string]bool{}}
	h := &SyncHandler{eventLog: el, gdprChecker: &mockGDPR{true}, auditSvc: &mockAudit{}, indexSvc: &mockIdx{}, feedGenerator: &mockFeed{}, log: logger.NewNop()}
	if err := h.processEvent(context.Background(), makeEvent(model.EventTypePostCreated, 900, 800, "hi"), "local_api"); err != nil { t.Fatal(err) }
	if !el.m["evt_test_001"] { t.Error("not marked") }
}
func TestProcessEvent_Idempotent(t *testing.T) {
	el := &mockEvt{m: map[string]bool{"evt_test_001": true}}
	h := &SyncHandler{eventLog: el, log: logger.NewNop()}
	if err := h.processEvent(context.Background(), makeEvent(model.EventTypePostCreated, 900, 800, "hi"), "x"); err != nil { t.Fatal(err) }
}
func TestProcessEvent_GDPRDenied(t *testing.T) {
	el := &mockEvt{m: map[string]bool{}}
	h := &SyncHandler{eventLog: el, gdprChecker: &mockGDPR{false}, auditSvc: &mockAudit{}, log: logger.NewNop()}
	if err := h.processEvent(context.Background(), makeEvent(model.EventTypePostCreated, 900, 800, "hi"), "x"); err != nil { t.Fatal(err) }
	if !el.m["evt_test_001"] { t.Error("marked") }
}
func TestProcessEvent_RouteError(t *testing.T) {
	el := &mockEvt{m: map[string]bool{}}
	h := &SyncHandler{eventLog: el, gdprChecker: &mockGDPR{true}, auditSvc: &mockAudit{}, indexSvc: &mockIdx{insErr: errors.New("db")}, log: logger.NewNop()}
	if err := h.processEvent(context.Background(), makeEvent(model.EventTypePostCreated, 900, 800, "hi"), "x"); err == nil { t.Fatal("expected err") }
}
func TestProcessEvent_MissingFields(t *testing.T) {
	h := &SyncHandler{log: logger.NewNop()}
	if err := h.processEvent(context.Background(), &model.CrossRegionSyncEvent{}, "x"); err == nil { t.Fatal("expected err") }
}

// ===== HandleSync/HandleCrossSync full pipeline =====

func TestHandleSync_Full_Success(t *testing.T) {
	el := &mockEvt{m: map[string]bool{}}
	h := &SyncHandler{eventLog: el, gdprChecker: &mockGDPR{true}, auditSvc: &mockAudit{}, indexSvc: &mockIdx{}, feedGenerator: &mockFeed{}, log: logger.NewNop()}
	r := chi.NewRouter(); r.Post("/sync/content", h.HandleSync)
	body := `{"eventId":"e1","eventType":"POST_CREATED","payload":{"postUid":1,"authorUid":2},"metadata":{"dataCategory":"TIER_2"}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/sync/content", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != 202 { t.Fatalf("status=%d", rec.Code) }
}
func TestHandleSync_GDPRDenied(t *testing.T) {
	el := &mockEvt{m: map[string]bool{}}
	h := &SyncHandler{eventLog: el, gdprChecker: &mockGDPR{false}, auditSvc: &mockAudit{}, log: logger.NewNop()}
	r := chi.NewRouter(); r.Post("/sync/content", h.HandleSync)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/sync/content", strings.NewReader(`{"eventId":"e1","eventType":"POST_CREATED","payload":{"postUid":1},"metadata":{"dataCategory":"TIER_1"}}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != 202 { t.Fatalf("status=%d", rec.Code) }
}
func TestHandleSync_RouteError(t *testing.T) {
	el := &mockEvt{m: map[string]bool{}}
	h := &SyncHandler{eventLog: el, gdprChecker: &mockGDPR{true}, auditSvc: &mockAudit{}, indexSvc: &mockIdx{insErr: errors.New("db")}, log: logger.NewNop()}
	r := chi.NewRouter(); r.Post("/sync/content", h.HandleSync)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/sync/content", strings.NewReader(`{"eventId":"e1","eventType":"POST_CREATED","payload":{"postUid":1},"metadata":{"dataCategory":"TIER_2"}}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != 500 { t.Fatalf("status=%d", rec.Code) }
}
func TestHandleCrossSync_Full_Success(t *testing.T) {
	el := &mockEvt{m: map[string]bool{}}
	h := &SyncHandler{eventLog: el, gdprChecker: &mockGDPR{true}, auditSvc: &mockAudit{}, indexSvc: &mockIdx{}, log: logger.NewNop()}
	r := chi.NewRouter(); r.Post("/sync/cross-sync", h.HandleCrossSync)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/sync/cross-sync", strings.NewReader(`{"eventId":"e1","eventType":"POST_UPDATED","payload":{"postUid":1},"metadata":{"dataCategory":"TIER_2"}}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != 202 { t.Fatalf("status=%d", rec.Code) }
}
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
	for _, tt := range []struct{ n, b string }{{"no eventId", `{"eventType":"X","payload":{"postUid":1}}`}, {"no eventType", `{"eventId":"e1","payload":{"postUid":1}}`}, {"empty", `{}`}} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/sync/content", strings.NewReader(tt.b))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(rec, req)
		if rec.Code != 400 { t.Errorf("%s: status=%d", tt.n, rec.Code) }
	}
}

// ===== HandleGetPost =====

func TestHandleGetPost_Found(t *testing.T) {
	mock, _ := pgxmock.NewPool(); defer mock.Close()
	now := time.Now()
	rows := pgxmock.NewRows([]string{"post_slug", "author_uid", "author_region", "content_preview", "visibility", "hashtags", "mentions", "media_urls_str", "likes_count", "comments_count", "shares_count", "views_count", "gdpr_compliant", "user_consent", "data_category", "created_at", "synced_at", "author_nickname", "author_avatar_url"}).AddRow(int64(12345), int64(67890), "SEA", "Hello", "GLOBAL", nil, nil, "", 0, 0, 0, 0, true, true, "TIER_2", now, now, nil, nil)
	mock.ExpectQuery("SELECT").WithArgs(int64(12345)).WillReturnRows(rows)
	h := &SyncHandler{indexSvc: service.NewGlobalIndexServiceWithDB(mock, logger.NewNop()), log: logger.NewNop()}
	r := chi.NewRouter(); r.Get("/index/posts/{uid}", h.HandleGetPost)
	rec := httptest.NewRecorder(); r.ServeHTTP(rec, httptest.NewRequest("GET", "/index/posts/12345", nil))
	if rec.Code != 200 { t.Errorf("status=%d", rec.Code) }
}
func TestHandleGetPost_NotFound(t *testing.T) {
	mock, _ := pgxmock.NewPool(); defer mock.Close()
	mock.ExpectQuery("SELECT").WithArgs(int64(99999)).WillReturnError(pgx.ErrNoRows)
	h := &SyncHandler{indexSvc: service.NewGlobalIndexServiceWithDB(mock, logger.NewNop()), log: logger.NewNop()}
	r := chi.NewRouter(); r.Get("/index/posts/{uid}", h.HandleGetPost)
	rec := httptest.NewRecorder(); r.ServeHTTP(rec, httptest.NewRequest("GET", "/index/posts/99999", nil))
	if rec.Code != 404 { t.Errorf("status=%d", rec.Code) }
}
func TestHandleGetPost_InvalidUid(t *testing.T) {
	h := &SyncHandler{log: logger.NewNop()}
	r := chi.NewRouter(); r.Get("/index/posts/{uid}", h.HandleGetPost)
	for _, uid := range []string{"abc", "-5", "12a34"} {
		rec := httptest.NewRecorder(); r.ServeHTTP(rec, httptest.NewRequest("GET", "/index/posts/"+uid, nil))
		if rec.Code != 400 { t.Errorf("uid=%q status=%d", uid, rec.Code) }
	}
}

// ===== HandleGetPostByUid =====

func TestHandleGetPostByUid_Found(t *testing.T) {
	mock, _ := pgxmock.NewPool(); defer mock.Close()
	now := time.Now()
	rows := pgxmock.NewRows([]string{"post_slug", "author_uid", "author_region", "content_preview", "visibility", "hashtags", "mentions", "media_urls_str", "likes_count", "comments_count", "shares_count", "views_count", "gdpr_compliant", "user_consent", "data_category", "created_at", "synced_at", "author_nickname", "author_avatar_url"}).AddRow(int64(888), int64(777), "SEA", "test", "GLOBAL", nil, nil, "", 0, 0, 0, 0, true, true, "TIER_2", now, now, nil, nil)
	mock.ExpectQuery("SELECT").WithArgs(int64(888)).WillReturnRows(rows)
	h := &SyncHandler{indexSvc: service.NewGlobalIndexServiceWithDB(mock, logger.NewNop()), log: logger.NewNop()}
	r := chi.NewRouter(); r.Get("/index/posts/uid/{uid}", h.HandleGetPostByUid)
	rec := httptest.NewRecorder(); r.ServeHTTP(rec, httptest.NewRequest("GET", "/index/posts/uid/888", nil))
	if rec.Code != 200 { t.Errorf("status=%d", rec.Code) }
}
func TestHandleGetPostByUid_InvalidUid(t *testing.T) {
	h := &SyncHandler{log: logger.NewNop()}
	r := chi.NewRouter(); r.Get("/index/posts/uid/{uid}", h.HandleGetPostByUid)
	rec := httptest.NewRecorder(); r.ServeHTTP(rec, httptest.NewRequest("GET", "/index/posts/uid/abc", nil))
	if rec.Code != 400 { t.Errorf("status=%d", rec.Code) }
}

// ===== Tag endpoints =====

func TestHandleSearchTags(t *testing.T) {
	h := &SyncHandler{tagIndexSvc: &mockTag{}, log: logger.NewNop()}
	r := chi.NewRouter(); r.Get("/index/tags/search", h.HandleSearchTags)
	rec := httptest.NewRecorder(); r.ServeHTTP(rec, httptest.NewRequest("GET", "/index/tags/search?keyword=go&limit=5", nil))
	if rec.Code != 200 { t.Errorf("status=%d", rec.Code) }
}
func TestHandlePopularTags(t *testing.T) {
	h := &SyncHandler{tagIndexSvc: &mockTag{}, log: logger.NewNop()}
	r := chi.NewRouter(); r.Get("/index/tags/popular", h.HandlePopularTags)
	rec := httptest.NewRecorder(); r.ServeHTTP(rec, httptest.NewRequest("GET", "/index/tags/popular?limit=10", nil))
	if rec.Code != 200 { t.Errorf("status=%d", rec.Code) }
}
func TestHandleGetTag_Found(t *testing.T) {
	tg := &mockTag{tag: &model.GlobalTagIndex{TagUID: 700, Name: "golang"}}
	h := &SyncHandler{tagIndexSvc: tg, log: logger.NewNop()}
	r := chi.NewRouter(); r.Get("/index/tags/{tagUid}", h.HandleGetTag)
	rec := httptest.NewRecorder(); r.ServeHTTP(rec, httptest.NewRequest("GET", "/index/tags/700", nil))
	if rec.Code != 200 { t.Errorf("status=%d", rec.Code) }
}
func TestHandleGetTag_NotFound(t *testing.T) {
	h := &SyncHandler{tagIndexSvc: &mockTag{}, log: logger.NewNop()}
	r := chi.NewRouter(); r.Get("/index/tags/{tagUid}", h.HandleGetTag)
	rec := httptest.NewRecorder(); r.ServeHTTP(rec, httptest.NewRequest("GET", "/index/tags/999", nil))
	if rec.Code != 404 { t.Errorf("status=%d", rec.Code) }
}
func TestHandleGetTagRegions(t *testing.T) {
	tg := &mockTag{regions: []string{"SEA", "EU"}}
	h := &SyncHandler{tagIndexSvc: tg, log: logger.NewNop()}
	r := chi.NewRouter(); r.Get("/index/tags/{tagUid}/regions", h.HandleGetTagRegions)
	rec := httptest.NewRecorder(); r.ServeHTTP(rec, httptest.NewRequest("GET", "/index/tags/700/regions", nil))
	if rec.Code != 200 { t.Errorf("status=%d", rec.Code) }
}
