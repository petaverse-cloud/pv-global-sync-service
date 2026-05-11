package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/service"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

// ===== NewUserIndexReconciler =====

func TestNewUserIndexReconciler_EmptyPeerURL(t *testing.T) {
	r := NewUserIndexReconciler(nil, "", logger.NewNop(), time.Minute)
	if r != nil {
		t.Error("expected nil for empty peerURL")
	}
}

func TestNewUserIndexReconciler_ValidPeerURL(t *testing.T) {
	r := NewUserIndexReconciler(nil, "https://peer.example.com", logger.NewNop(), time.Minute)
	if r == nil {
		t.Fatal("expected non-nil for valid peerURL")
	}
	if r.interval != time.Minute {
		t.Errorf("interval=%v", r.interval)
	}
}

// ===== fetchPeerEntries =====

func TestFetchPeerEntries_Success(t *testing.T) {
	// Mock HTTP server returning valid user index entries
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/index/users/all" {
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"users": []map[string]interface{}{
				{"uid": 100, "region": "SEA", "emailHash": "abc"},
				{"uid": 200, "region": "EU", "emailHash": nil},
			},
		})
	}))
	defer srv.Close()

	r := &UserIndexReconciler{peerURL: srv.URL, httpCli: srv.Client(), log: logger.NewNop()}
	entries, err := r.fetchPeerEntries(context.Background())
	if err != nil {
		t.Fatalf("fetchPeerEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len=%d want 2", len(entries))
	}
	if entries[0].UID != 100 || entries[0].Region != "SEA" {
		t.Errorf("entry[0]=%+v", entries[0])
	}
	if entries[1].EmailHash != nil {
		t.Error("expected nil EmailHash for OAuth user")
	}
}

func TestFetchPeerEntries_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	r := &UserIndexReconciler{peerURL: srv.URL, httpCli: srv.Client(), log: logger.NewNop()}
	_, err := r.fetchPeerEntries(context.Background())
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestFetchPeerEntries_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	r := &UserIndexReconciler{peerURL: srv.URL, httpCli: srv.Client(), log: logger.NewNop()}
	_, err := r.fetchPeerEntries(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestFetchPeerEntries_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"users": []interface{}{}})
	}))
	defer srv.Close()

	r := &UserIndexReconciler{peerURL: srv.URL, httpCli: srv.Client(), log: logger.NewNop()}
	entries, err := r.fetchPeerEntries(context.Background())
	if err != nil {
		t.Fatalf("fetchPeerEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("len=%d want 0", len(entries))
	}
}

func TestFetchPeerEntries_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // longer than client timeout
	}))
	defer srv.Close()

	r := &UserIndexReconciler{
		peerURL: srv.URL,
		httpCli: &http.Client{Timeout: 50 * time.Millisecond},
		log:     logger.NewNop(),
	}
	_, err := r.fetchPeerEntries(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// ===== reconcile =====

func TestReconcile_SyncMissing(t *testing.T) {
	mockDB, _ := pgxmock.NewPool()
	defer mockDB.Close()
	svc := service.NewGlobalIndexServiceWithDB(mockDB, logger.NewNop())

	// Mock HTTP peer returns 2 users
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"users": []map[string]interface{}{
				{"uid": 100, "region": "SEA"},
				{"uid": 200, "region": "EU"},
			},
		})
	}))
	defer srv.Close()

	// Local has only uid 100
	localRows := pgxmock.NewRows([]string{"uid", "email_hash", "region"}).
		AddRow(int64(100), nil, "SEA")
	mockDB.ExpectQuery("SELECT uid, email_hash, region FROM users_global_index").WillReturnRows(localRows)

	// Should upsert uid 200 (missing locally)
	mockDB.ExpectExec("INSERT INTO users_global_index").
		WithArgs(int64(200), "EU", (*string)(nil)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	r := &UserIndexReconciler{
		indexSvc: svc, peerURL: srv.URL, httpCli: srv.Client(), log: logger.NewNop(),
	}
	r.reconcile(context.Background())

	if err := mockDB.ExpectationsWereMet(); err != nil {
		t.Errorf("DB expectations: %v", err)
	}
}

func TestReconcile_NoMissing(t *testing.T) {
	mockDB, _ := pgxmock.NewPool()
	defer mockDB.Close()
	svc := service.NewGlobalIndexServiceWithDB(mockDB, logger.NewNop())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"users": []map[string]interface{}{{"uid": 100, "region": "SEA"}},
		})
	}))
	defer srv.Close()

	localRows := pgxmock.NewRows([]string{"uid", "email_hash", "region"}).
		AddRow(int64(100), nil, "SEA")
	mockDB.ExpectQuery("SELECT uid, email_hash, region FROM users_global_index").WillReturnRows(localRows)
	// No Exec expected — no missing entries

	r := &UserIndexReconciler{
		indexSvc: svc, peerURL: srv.URL, httpCli: srv.Client(), log: logger.NewNop(),
	}
	r.reconcile(context.Background())

	if err := mockDB.ExpectationsWereMet(); err != nil {
		t.Errorf("DB expectations: %v", err)
	}
}

func TestReconcile_PeerUnreachable_GracefulDegradation(t *testing.T) {
	mockDB, _ := pgxmock.NewPool()
	defer mockDB.Close()
	svc := service.NewGlobalIndexServiceWithDB(mockDB, logger.NewNop())

	// Peer server that always fails
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	r := &UserIndexReconciler{
		indexSvc: svc, peerURL: srv.URL, httpCli: srv.Client(), log: logger.NewNop(),
	}
	// Should not panic — just log warning and return
	r.reconcile(context.Background())

	// No DB calls should have been made (fetch failed before DB query)
	if err := mockDB.ExpectationsWereMet(); err != nil {
		t.Errorf("DB expectations (should be none): %v", err)
	}
}

func TestReconcile_LocalDBError_Graceful(t *testing.T) {
	mockDB, _ := pgxmock.NewPool()
	defer mockDB.Close()
	svc := service.NewGlobalIndexServiceWithDB(mockDB, logger.NewNop())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"users": []map[string]interface{}{{"uid": 100, "region": "SEA"}},
		})
	}))
	defer srv.Close()

	mockDB.ExpectQuery("SELECT uid, email_hash, region FROM users_global_index").
		WillReturnError(fmt.Errorf("db connection closed"))

	r := &UserIndexReconciler{
		indexSvc: svc, peerURL: srv.URL, httpCli: srv.Client(), log: logger.NewNop(),
	}
	// Should not panic — just log error and return
	r.reconcile(context.Background())
}
