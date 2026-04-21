package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/peer"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/sync"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

// TestSyncHandler_HandleCrossSync_InvalidJSON verifies input validation
// before processEvent is called (no nil deps needed).
func TestSyncHandler_HandleCrossSync_InvalidJSON(t *testing.T) {
	log, _ := logger.New("warn", "console")
	pm := peer.NewPeerManager([]string{}, 100*time.Millisecond)
	crossSync := sync.NewCrossSyncService(pm, 100*time.Millisecond, log)

	h := &SyncHandler{
		crossSync: crossSync,
		log:       log,
	}

	req := httptest.NewRequest(http.MethodPost, "/sync/cross-sync", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	h.HandleCrossSync(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleCrossSync() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestSyncHandler_HandleCrossSync_MethodNotAllowed verifies HTTP method check.
func TestSyncHandler_HandleCrossSync_MethodNotAllowed(t *testing.T) {
	log, _ := logger.New("warn", "console")
	pm := peer.NewPeerManager([]string{}, 100*time.Millisecond)
	crossSync := sync.NewCrossSyncService(pm, 100*time.Millisecond, log)

	h := &SyncHandler{
		crossSync: crossSync,
		log:       log,
	}

	req := httptest.NewRequest(http.MethodGet, "/sync/cross-sync", nil)
	rec := httptest.NewRecorder()

	h.HandleCrossSync(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("HandleCrossSync() status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// TestSyncHandler_HandleSync_InvalidJSON verifies input validation.
func TestSyncHandler_HandleSync_InvalidJSON(t *testing.T) {
	log, _ := logger.New("warn", "console")

	h := &SyncHandler{
		log: log,
	}

	req := httptest.NewRequest(http.MethodPost, "/sync/content", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	h.HandleSync(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleSync() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// HandleSync – missing required fields (returns 400 before processEvent)
// ---------------------------------------------------------------------------

func TestSyncHandler_HandleSync_MissingEventID(t *testing.T) {
	log, _ := logger.New("warn", "console")
	h := &SyncHandler{log: log}

	body := `{"eventType":"POST_CREATED","payload":{}}`
	req := httptest.NewRequest(http.MethodPost, "/sync/content", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleSync(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleSync() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSyncHandler_HandleSync_MissingEventType(t *testing.T) {
	log, _ := logger.New("warn", "console")
	h := &SyncHandler{log: log}

	body := `{"eventId":"evt-001","payload":{}}`
	req := httptest.NewRequest(http.MethodPost, "/sync/content", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleSync(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleSync() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSyncHandler_HandleSync_MissingBothFields(t *testing.T) {
	log, _ := logger.New("warn", "console")
	h := &SyncHandler{log: log}

	body := `{"payload":{}}`
	req := httptest.NewRequest(http.MethodPost, "/sync/content", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleSync(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleSync() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSyncHandler_HandleSync_MethodNotAllowed(t *testing.T) {
	log, _ := logger.New("warn", "console")
	h := &SyncHandler{log: log}

	req := httptest.NewRequest(http.MethodGet, "/sync/content", nil)
	rec := httptest.NewRecorder()

	h.HandleSync(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("HandleSync() status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// HandleCrossSync – missing required fields (returns 400 before processEvent)
// ---------------------------------------------------------------------------

func TestSyncHandler_HandleCrossSync_MissingEventID(t *testing.T) {
	log, _ := logger.New("warn", "console")
	pm := peer.NewPeerManager([]string{}, 100*time.Millisecond)
	crossSync := sync.NewCrossSyncService(pm, 100*time.Millisecond, log)
	h := &SyncHandler{crossSync: crossSync, log: log}

	body := `{"eventType":"POST_CREATED","payload":{}}`
	req := httptest.NewRequest(http.MethodPost, "/sync/cross-sync", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleCrossSync(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleCrossSync() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSyncHandler_HandleCrossSync_MissingEventType(t *testing.T) {
	log, _ := logger.New("warn", "console")
	pm := peer.NewPeerManager([]string{}, 100*time.Millisecond)
	crossSync := sync.NewCrossSyncService(pm, 100*time.Millisecond, log)
	h := &SyncHandler{crossSync: crossSync, log: log}

	body := `{"eventId":"evt-002","payload":{}}`
	req := httptest.NewRequest(http.MethodPost, "/sync/cross-sync", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleCrossSync(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleCrossSync() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSyncHandler_HandleCrossSync_MissingBothFields(t *testing.T) {
	log, _ := logger.New("warn", "console")
	pm := peer.NewPeerManager([]string{}, 100*time.Millisecond)
	crossSync := sync.NewCrossSyncService(pm, 100*time.Millisecond, log)
	h := &SyncHandler{crossSync: crossSync, log: log}

	body := `{"payload":{}}`
	req := httptest.NewRequest(http.MethodPost, "/sync/cross-sync", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleCrossSync(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleCrossSync() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// HandleGetPost – missing / invalid postId (returns 400 before indexSvc)
// ---------------------------------------------------------------------------

func TestSyncHandler_HandleGetPost_MissingPostID(t *testing.T) {
	log, _ := logger.New("warn", "console")
	h := &SyncHandler{log: log}

	req := httptest.NewRequest(http.MethodGet, "/index/posts/", nil)
	// chi router would normally populate the URL param; simulate empty param
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{Keys: []string{"postId"}, Values: []string{""}},
	}))
	rec := httptest.NewRecorder()

	h.HandleGetPost(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleGetPost() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSyncHandler_HandleGetPost_InvalidPostID_NonNumeric(t *testing.T) {
	log, _ := logger.New("warn", "console")
	h := &SyncHandler{log: log}

	req := httptest.NewRequest(http.MethodGet, "/index/posts/abc", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{Keys: []string{"postId"}, Values: []string{"abc"}},
	}))
	rec := httptest.NewRecorder()

	h.HandleGetPost(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleGetPost() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSyncHandler_HandleGetPost_InvalidPostID_Negative(t *testing.T) {
	log, _ := logger.New("warn", "console")
	h := &SyncHandler{log: log}

	req := httptest.NewRequest(http.MethodGet, "/index/posts/-5", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{Keys: []string{"postId"}, Values: []string{"-5"}},
	}))
	rec := httptest.NewRecorder()

	h.HandleGetPost(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleGetPost() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSyncHandler_HandleGetPost_InvalidPostID_MixedChars(t *testing.T) {
	log, _ := logger.New("warn", "console")
	h := &SyncHandler{log: log}

	req := httptest.NewRequest(http.MethodGet, "/index/posts/12a34", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{Keys: []string{"postId"}, Values: []string{"12a34"}},
	}))
	rec := httptest.NewRecorder()

	h.HandleGetPost(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("HandleGetPost() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// parseInt64 – unit tests (same package, directly callable)
// ---------------------------------------------------------------------------

func TestParseInt64_ValidZero(t *testing.T) {
	n, err := parseInt64("0")
	if err != nil {
		t.Errorf("parseInt64(\"0\") unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("parseInt64(\"0\") = %d, want 0", n)
	}
}

func TestParseInt64_ValidPositive(t *testing.T) {
	n, err := parseInt64("12345")
	if err != nil {
		t.Errorf("parseInt64(\"12345\") unexpected error: %v", err)
	}
	if n != 12345 {
		t.Errorf("parseInt64(\"12345\") = %d, want 12345", n)
	}
}

func TestParseInt64_ValidLarge(t *testing.T) {
	n, err := parseInt64("9223372036854775807") // max int64
	if err != nil {
		t.Errorf("parseInt64(max int64) unexpected error: %v", err)
	}
	if n != 9223372036854775807 {
		t.Errorf("parseInt64(max int64) = %d, want 9223372036854775807", n)
	}
}

func TestParseInt64_ValidSingleDigit(t *testing.T) {
	n, err := parseInt64("7")
	if err != nil {
		t.Errorf("parseInt64(\"7\") unexpected error: %v", err)
	}
	if n != 7 {
		t.Errorf("parseInt64(\"7\") = %d, want 7", n)
	}
}

func TestParseInt64_EmptyString(t *testing.T) {
	// parseInt64 does NOT treat empty string as an error.
	// The for-loop simply doesn't execute, returning (0, nil).
	n, err := parseInt64("")
	if err != nil {
		t.Errorf("parseInt64(\"\") unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("parseInt64(\"\") = %d, want 0", n)
	}
}

func TestParseInt64_NegativeSign(t *testing.T) {
	n, err := parseInt64("-5")
	if err == nil {
		t.Error("parseInt64(\"-5\") expected error, got nil")
	}
	if n != 0 {
		t.Errorf("parseInt64(\"-5\") = %d, want 0", n)
	}
}

func TestParseInt64_NonDigitChars(t *testing.T) {
	testCases := []struct {
		input string
		name  string
	}{
		{"abc", "letters_only"},
		{"12a34", "mixed"},
		{"12.34", "decimal"},
		{" 42", "leading_space"},
		{"42 ", "trailing_space"},
		{"+42", "plus_sign"},
		{"0x1F", "hex"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			n, err := parseInt64(tc.input)
			if err == nil {
				t.Errorf("parseInt64(%q) expected error, got nil (n=%d)", tc.input, n)
			}
		})
	}
}

func TestParseInt64_Overflow(t *testing.T) {
	// "99999999999999999999" exceeds max int64 (9223372036854775807).
	// parseInt64 does NOT detect overflow; it silently wraps.
	n, err := parseInt64("99999999999999999999")
	if err != nil {
		t.Errorf("parseInt64 overflow: expected no error (function has no overflow guard), got %v", err)
	}
	// Verify the value is NOT equal to the true mathematical value by checking
	// that it differs from max int64 (the correct value would be > max int64).
	if n == 9223372036854775807 {
		t.Error("parseInt64 overflow: wrapped value should not equal max int64")
	}
	// Document the actual wrapped behavior:
	t.Logf("parseInt64(\"99999999999999999999\") wrapped to %d (int64 overflow, no error returned)", n)
}

func TestParseInt64_OverflowOneOverMax(t *testing.T) {
	// max int64 + 1  =  9223372036854775808
	n, err := parseInt64("9223372036854775808")
	if err != nil {
		t.Errorf("parseInt64(max+1) expected no error (no overflow guard), got %v", err)
	}
	// The value will wrap to negative (or some other value) due to int64 overflow.
	// It should NOT equal max int64.
	if n == 9223372036854775807 {
		t.Error("parseInt64(max+1) should have wrapped, not equal to max int64")
	}
	t.Logf("parseInt64(\"9223372036854775808\") wrapped to %d", n)
}

// ---------------------------------------------------------------------------
// writeError – unit tests (same package, directly callable)
// ---------------------------------------------------------------------------

func TestWriteError_StatusBadRequest(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "invalid input")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("writeError status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("writeError Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestWriteError_StatusNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusNotFound, "resource not found")

	if rec.Code != http.StatusNotFound {
		t.Errorf("writeError status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("writeError Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestWriteError_StatusInternalServerError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusInternalServerError, "db error")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("writeError status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("writeError Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestWriteError_StatusMethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusMethodNotAllowed, "method not allowed")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("writeError status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("writeError Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestWriteError_StatusAccepted(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusAccepted, "accepted")

	if rec.Code != http.StatusAccepted {
		t.Errorf("writeError status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("writeError Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestWriteError_JSONBodyStructure(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "test error message")

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("writeError body is not valid JSON: %v", err)
	}
	if body["error"] != "Bad Request" {
		t.Errorf("writeError body[\"error\"] = %q, want %q", body["error"], "Bad Request")
	}
	if body["message"] != "test error message" {
		t.Errorf("writeError body[\"message\"] = %q, want %q", body["message"], "test error message")
	}
}

func TestWriteError_ContentTypeAlwaysJSON(t *testing.T) {
	statuses := []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusInternalServerError,
		http.StatusServiceUnavailable,
	}
	for _, status := range statuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeError(rec, status, "error")
			ct := rec.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("writeError(%d) Content-Type = %q, want %q", status, ct, "application/json")
			}
		})
	}
}

func TestWriteError_EmptyMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "")

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("writeError body is not valid JSON: %v", err)
	}
	if body["message"] != "" {
		t.Errorf("writeError body[\"message\"] = %q, want empty string", body["message"])
	}
}
