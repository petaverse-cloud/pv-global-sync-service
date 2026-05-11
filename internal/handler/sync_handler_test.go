package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/peer"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/service"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/sync"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

func newNopLog() *logger.Logger { return logger.NewNop() }
func ptrStr(s string) *string  { return &s }

func setupGetPostHandler(mock pgxmock.PgxPoolIface) *SyncHandler {
	return &SyncHandler{
		indexSvc: service.NewGlobalIndexServiceWithDB(mock, newNopLog()),
		log:      newNopLog(),
	}
}

// ============================================
// HandleGetPost tests
// ============================================

func TestHandleGetPost_Found(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	h := setupGetPostHandler(mock)
	now := time.Now().UTC()

	rows := pgxmock.NewRows([]string{
		"post_slug", "author_uid", "author_region", "content_preview", "visibility",
		"hashtags", "mentions", "media_urls_str",
		"likes_count", "comments_count", "shares_count", "views_count",
		"gdpr_compliant", "user_consent", "data_category", "created_at", "synced_at",
		"author_nickname", "author_avatar_url",
	}).AddRow(int64(12345), int64(67890), "SEA", "Hello", "GLOBAL",
		[]byte("{test}"), []byte("{1}"), "https://a.jpg",
		5, 3, 1, 100, true, true, "TIER_2", now, now,
		ptrStr("TestAuthor"), ptrStr("https://cdn.example.com/a.jpg"))

	mock.ExpectQuery("SELECT").WithArgs(int64(12345)).WillReturnRows(rows)

	r := chi.NewRouter()
	r.Get("/index/posts/{uid}", h.HandleGetPost)
	req := httptest.NewRequest("GET", "/index/posts/12345", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	var post model.GlobalPostIndex
	json.Unmarshal(rec.Body.Bytes(), &post)
	if post.PostUid != 12345 {
		t.Errorf("PostUid=%d", post.PostUid)
	}
}

func TestHandleGetPost_NotFound(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	h := setupGetPostHandler(mock)

	mock.ExpectQuery("SELECT").WithArgs(int64(99999)).WillReturnError(pgx.ErrNoRows)

	r := chi.NewRouter()
	r.Get("/index/posts/{uid}", h.HandleGetPost)
	req := httptest.NewRequest("GET", "/index/posts/99999", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

func TestHandleGetPost_InvalidUid(t *testing.T) {
	h := &SyncHandler{log: newNopLog()}
	r := chi.NewRouter()
	r.Get("/index/posts/{uid}", h.HandleGetPost)

	for _, uid := range []string{"abc", "-5", "12a34"} {
		req := httptest.NewRequest("GET", "/index/posts/"+uid, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != 400 {
			t.Errorf("uid=%q status=%d want 400", uid, rec.Code)
		}
	}
}

func TestHandleGetPostByUid_Found(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	h := setupGetPostHandler(mock)
	now := time.Now().UTC()

	rows := pgxmock.NewRows([]string{
		"post_slug", "author_uid", "author_region", "content_preview", "visibility",
		"hashtags", "mentions", "media_urls_str",
		"likes_count", "comments_count", "shares_count", "views_count",
		"gdpr_compliant", "user_consent", "data_category", "created_at", "synced_at",
		"author_nickname", "author_avatar_url",
	}).AddRow(int64(55555), int64(11111), "EU", "By uid", "REGIONAL",
		nil, nil, "", 0, 0, 0, 0, false, false, "TIER_3", now, now, nil, nil)

	mock.ExpectQuery("SELECT").WithArgs(int64(55555)).WillReturnRows(rows)

	r := chi.NewRouter()
	r.Get("/index/posts/uid/{uid}", h.HandleGetPostByUid)
	req := httptest.NewRequest("GET", "/index/posts/uid/55555", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestHandleGetPostByUid_InvalidUid(t *testing.T) {
	h := &SyncHandler{log: newNopLog()}
	r := chi.NewRouter()
	r.Get("/index/posts/uid/{uid}", h.HandleGetPostByUid)
	req := httptest.NewRequest("GET", "/index/posts/uid/abc", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Errorf("status=%d want 400", rec.Code)
	}
}

// ============================================
// HandleSync validation tests (no full pipeline)
// ============================================

func TestHandleSync_InvalidJSON(t *testing.T) {
	h := &SyncHandler{log: newNopLog()}
	r := chi.NewRouter()
	r.Post("/sync/content", h.HandleSync)
	req := httptest.NewRequest("POST", "/sync/content", strings.NewReader("bad json"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Errorf("status=%d want 400", rec.Code)
	}
}

func TestHandleSync_MissingFields(t *testing.T) {
	h := &SyncHandler{log: newNopLog()}
	r := chi.NewRouter()
	r.Post("/sync/content", h.HandleSync)

	tests := []struct{ name, body string }{
		{"no eventId", `{"eventType":"POST_CREATED","payload":{"postUid":1,"authorUid":2}}`},
		{"no eventType", `{"eventId":"evt_1","payload":{"postUid":1,"authorUid":2}}`},
		{"empty", `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/sync/content", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != 400 {
				t.Errorf("status=%d want 400", rec.Code)
			}
		})
	}
}

func TestHandleSync_MethodNotAllowed(t *testing.T) {
	h := &SyncHandler{log: newNopLog()}
	r := chi.NewRouter()
	r.Post("/sync/content", h.HandleSync)
	req := httptest.NewRequest("GET", "/sync/content", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 405 {
		t.Errorf("status=%d want 405", rec.Code)
	}
}

func TestHandleCrossSync_InvalidJSON(t *testing.T) {
	pm := peer.NewPeerManager([]string{}, 100*time.Millisecond)
	crossSyncSvc := sync.NewCrossSyncService(pm, 100*time.Millisecond, newNopLog())
	h := &SyncHandler{crossSync: crossSyncSvc, log: newNopLog()}

	r := chi.NewRouter()
	r.Post("/sync/cross-sync", h.HandleCrossSync)
	req := httptest.NewRequest("POST", "/sync/cross-sync", strings.NewReader("bad"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Errorf("status=%d want 400", rec.Code)
	}
}

func TestHandleCrossSync_MethodNotAllowed(t *testing.T) {
	pm := peer.NewPeerManager([]string{}, 100*time.Millisecond)
	crossSyncSvc := sync.NewCrossSyncService(pm, 100*time.Millisecond, newNopLog())
	h := &SyncHandler{crossSync: crossSyncSvc, log: newNopLog()}

	r := chi.NewRouter()
	r.Post("/sync/cross-sync", h.HandleCrossSync)
	req := httptest.NewRequest("GET", "/sync/cross-sync", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 405 {
		t.Errorf("status=%d want 405", rec.Code)
	}
}
