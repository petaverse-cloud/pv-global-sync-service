package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/service"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

// mockFeedGenerator implements the FeedGenerator interface for testing.
type mockFeedGenerator struct {
	items      []service.FeedItem
	nextCursor string
	hasMore    bool
	err        error
	// captured arguments for assertion
	capturedUserID   int64
	capturedFeedType string
	capturedCursor   string
	capturedLimit    int
	callCount        int
}

func (m *mockFeedGenerator) GetFeed(ctx context.Context, userID int64, feedType string, cursor string, limit int) ([]service.FeedItem, string, bool, error) {
	m.callCount++
	m.capturedUserID = userID
	m.capturedFeedType = feedType
	m.capturedCursor = cursor
	m.capturedLimit = limit
	return m.items, m.nextCursor, m.hasMore, m.err
}

func setupTestRouter(handler http.HandlerFunc) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/feed/{userId}", handler)
	return r
}

func makeRequest(router *chi.Mux, method, url string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func decodeResponse(t *testing.T, rr *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return body
}

func decodeErrorResponse(t *testing.T, rr *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode error response body: %v", err)
	}
	return body
}

// ---------------------------------------------------------------------------
// Invalid userId tests (return 400 before GetFeed is called)
// ---------------------------------------------------------------------------

func TestHandleGetFeed_InvalidUserId_Empty(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{}
	h := NewFeedHandler(mock, log)

	// Empty userId doesn't match the chi route pattern /feed/{userId},
	// so the router returns 404. This is expected behavior.
	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/")
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404 (route not matched for empty userId), got %d", rr.Code)
	}
	if mock.callCount > 0 {
		t.Error("GetFeed should not be called for empty userId")
	}
}

func TestHandleGetFeed_InvalidUserId_NonNumeric(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/abc")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
	if mock.callCount > 0 {
		t.Error("GetFeed should not be called for non-numeric userId")
	}
}

func TestHandleGetFeed_InvalidUserId_Float(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/12.5")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
	if mock.callCount > 0 {
		t.Error("GetFeed should not be called for float userId")
	}
}

func TestHandleGetFeed_InvalidUserId_Negative(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	// Negative numbers are valid int64 values, so the handler accepts them.
	// This tests that ParseInt handles negative numbers correctly.
	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/-1")
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d (negative is valid int64), got %d", http.StatusOK, rr.Code)
	}
	if mock.callCount != 1 {
		t.Errorf("GetFeed should be called once for negative userId, got %d calls", mock.callCount)
	}
	if mock.capturedUserID != -1 {
		t.Errorf("expected userID -1, got %d", mock.capturedUserID)
	}
}

func TestHandleGetFeed_InvalidUserId_Zero(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/0")
	// 0 is a valid int64, so this should pass userId parsing
	if rr.Code == http.StatusBadRequest {
		t.Error("userId=0 should pass integer parsing")
	}
	if mock.callCount != 1 {
		t.Errorf("GetFeed should be called once for userId=0, got %d calls", mock.callCount)
	}
}

func TestHandleGetFeed_ValidUserId_CallsGetFeed(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{
		items:      []service.FeedItem{},
		nextCursor: "",
		hasMore:    false,
		err:        nil,
	}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/42")
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if mock.callCount != 1 {
		t.Errorf("GetFeed should be called exactly once, got %d calls", mock.callCount)
	}
	if mock.capturedUserID != 42 {
		t.Errorf("expected userID 42, got %d", mock.capturedUserID)
	}
}

func TestHandleGetFeed_ValidUserId_LargeNumber(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{
		items: []service.FeedItem{},
	}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/9223372036854775807")
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if mock.capturedUserID != 9223372036854775807 {
		t.Errorf("expected userID 9223372036854775807, got %d", mock.capturedUserID)
	}
}

// ---------------------------------------------------------------------------
// Query param default tests
// ---------------------------------------------------------------------------

func TestHandleGetFeed_DefaultFeedType(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if mock.capturedFeedType != "following" {
		t.Errorf("expected default feedType 'following', got '%s'", mock.capturedFeedType)
	}
}

func TestHandleGetFeed_DefaultLimit(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if mock.capturedLimit != 20 {
		t.Errorf("expected default limit 20, got %d", mock.capturedLimit)
	}

	// Verify limit appears in response metadata
	body := decodeResponse(t, rr)
	meta, ok := body["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("response missing metadata field")
	}
	if meta["limit"] != float64(20) {
		t.Errorf("expected metadata.limit 20, got %v", meta["limit"])
	}
}

func TestHandleGetFeed_Defaults_Combined(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify response metadata reflects defaults
	body := decodeResponse(t, rr)
	meta, ok := body["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("response missing metadata field")
	}
	if meta["feedType"] != "following" {
		t.Errorf("expected metadata.feedType 'following', got '%v'", meta["feedType"])
	}
	if meta["limit"] != float64(20) {
		t.Errorf("expected metadata.limit 20, got %v", meta["limit"])
	}
}

// ---------------------------------------------------------------------------
// Limit clamping and parsing tests
// ---------------------------------------------------------------------------

func TestHandleGetFeed_Limit_ClampedTo100(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1?limit=500")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if mock.capturedLimit != 100 {
		t.Errorf("expected limit clamped to 100, got %d", mock.capturedLimit)
	}

	body := decodeResponse(t, rr)
	meta := body["metadata"].(map[string]interface{})
	if meta["limit"] != float64(100) {
		t.Errorf("expected metadata.limit 100, got %v", meta["limit"])
	}
}

func TestHandleGetFeed_Limit_AtMax(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1?limit=100")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if mock.capturedLimit != 100 {
		t.Errorf("expected limit 100, got %d", mock.capturedLimit)
	}
}

func TestHandleGetFeed_Limit_CustomValue(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1?limit=5")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if mock.capturedLimit != 5 {
		t.Errorf("expected limit 5, got %d", mock.capturedLimit)
	}
}

func TestHandleGetFeed_Limit_Zero_Ignored(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1?limit=0")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	// limit=0 is not > 0, so it should be ignored and default to 20
	if mock.capturedLimit != 20 {
		t.Errorf("expected limit to default to 20 when 0 is given, got %d", mock.capturedLimit)
	}
}

func TestHandleGetFeed_Limit_Negative_Ignored(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1?limit=-10")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	// negative limit is not > 0, so it should be ignored
	if mock.capturedLimit != 20 {
		t.Errorf("expected limit to default to 20 when negative is given, got %d", mock.capturedLimit)
	}
}

func TestHandleGetFeed_Limit_NonNumeric_Ignored(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1?limit=abc")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if mock.capturedLimit != 20 {
		t.Errorf("expected limit to default to 20 when non-numeric given, got %d", mock.capturedLimit)
	}
}

func TestHandleGetFeed_Limit_Float_Ignored(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1?limit=10.5")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if mock.capturedLimit != 20 {
		t.Errorf("expected limit to default to 20 when float given, got %d", mock.capturedLimit)
	}
}

// ---------------------------------------------------------------------------
// feedType value tests
// ---------------------------------------------------------------------------

func TestHandleGetFeed_FeedType_Following(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1?feedType=following")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if mock.capturedFeedType != "following" {
		t.Errorf("expected feedType 'following', got '%s'", mock.capturedFeedType)
	}
}

func TestHandleGetFeed_FeedType_Global(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1?feedType=global")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if mock.capturedFeedType != "global" {
		t.Errorf("expected feedType 'global', got '%s'", mock.capturedFeedType)
	}
}

func TestHandleGetFeed_FeedType_Trending(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1?feedType=trending")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if mock.capturedFeedType != "trending" {
		t.Errorf("expected feedType 'trending', got '%s'", mock.capturedFeedType)
	}
}

func TestHandleGetFeed_FeedType_Unknown_FallsThrough(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1?feedType=unknown")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	// Unknown feedType is passed through as-is to GetFeed
	if mock.capturedFeedType != "unknown" {
		t.Errorf("expected feedType 'unknown' passed through, got '%s'", mock.capturedFeedType)
	}
}

func TestHandleGetFeed_FeedType_EmptyString_DefaultsToFollowing(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	// Explicitly passing feedType= should default to following
	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1?feedType=")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if mock.capturedFeedType != "following" {
		t.Errorf("expected feedType 'following' for empty string, got '%s'", mock.capturedFeedType)
	}
}

// ---------------------------------------------------------------------------
// Cursor parameter tests
// ---------------------------------------------------------------------------

func TestHandleGetFeed_Cursor_PassedThrough(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1?cursor=abc123")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if mock.capturedCursor != "abc123" {
		t.Errorf("expected cursor 'abc123', got '%s'", mock.capturedCursor)
	}
}

func TestHandleGetFeed_Cursor_Empty(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if mock.capturedCursor != "" {
		t.Errorf("expected empty cursor, got '%s'", mock.capturedCursor)
	}
}

// ---------------------------------------------------------------------------
// Combined query param tests
// ---------------------------------------------------------------------------

func TestHandleGetFeed_MultipleQueryParams(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/42?feedType=trending&limit=50&cursor=page2")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if mock.capturedUserID != 42 {
		t.Errorf("expected userID 42, got %d", mock.capturedUserID)
	}
	if mock.capturedFeedType != "trending" {
		t.Errorf("expected feedType 'trending', got '%s'", mock.capturedFeedType)
	}
	if mock.capturedLimit != 50 {
		t.Errorf("expected limit 50, got %d", mock.capturedLimit)
	}
	if mock.capturedCursor != "page2" {
		t.Errorf("expected cursor 'page2', got '%s'", mock.capturedCursor)
	}
}

func TestHandleGetFeed_ResponseStructure(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{
		items: []service.FeedItem{
			{PostID: 1, AuthorID: 10, Score: 0.95},
			{PostID: 2, AuthorID: 11, Score: 0.85},
		},
		nextCursor: "page2",
		hasMore:    true,
	}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	body := decodeResponse(t, rr)

	// Check items
	items, ok := body["items"].([]interface{})
	if !ok {
		t.Fatal("response missing items field or wrong type")
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}

	// Check nextCursor
	if body["nextCursor"] != "page2" {
		t.Errorf("expected nextCursor 'page2', got '%v'", body["nextCursor"])
	}

	// Check hasMore
	if body["hasMore"] != true {
		t.Errorf("expected hasMore true, got %v", body["hasMore"])
	}

	// Check metadata
	meta, ok := body["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("response missing metadata field")
	}
	if meta["feedType"] != "following" {
		t.Errorf("expected metadata.feedType 'following', got '%v'", meta["feedType"])
	}
	if meta["limit"] != float64(20) {
		t.Errorf("expected metadata.limit 20, got %v", meta["limit"])
	}
}

// ---------------------------------------------------------------------------
// GetFeed error handling tests
// ---------------------------------------------------------------------------

func TestHandleGetFeed_GeneratorError_Returns500(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{
		err: context.DeadlineExceeded,
	}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1")
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
	errBody := decodeErrorResponse(t, rr)
	if errBody["message"] != "failed to generate feed" {
		t.Errorf("expected error message 'failed to generate feed', got '%s'", errBody["message"])
	}
}

func TestHandleGetFeed_GeneratorError_EmptyItems(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{
		items:   []service.FeedItem{},
		hasMore: false,
		err:     nil,
	}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1")
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	body := decodeResponse(t, rr)
	items, ok := body["items"].([]interface{})
	if !ok {
		t.Fatal("response missing items field")
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

// ---------------------------------------------------------------------------
// Response content-type test
// ---------------------------------------------------------------------------

func TestHandleGetFeed_ContentType_JSON(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{items: []service.FeedItem{}}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type to contain 'application/json', got '%s'", ct)
	}
}

func TestHandleGetFeed_Error_ContentType_JSON(t *testing.T) {
	log, _ := logger.New("warn", "console")
	mock := &mockFeedGenerator{err: context.DeadlineExceeded}
	h := NewFeedHandler(mock, log)

	rr := makeRequest(setupTestRouter(h.HandleGetFeed), "GET", "/feed/1")
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type to contain 'application/json', got '%s'", ct)
	}
}
