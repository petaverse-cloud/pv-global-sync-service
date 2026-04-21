package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockChecker implements Checker interface for tests.
type mockChecker struct {
	fail bool
}

func (m *mockChecker) Ping(ctx context.Context) error {
	if m.fail {
		return context.DeadlineExceeded
	}
	return nil
}

func TestRegister_MountsHandlers(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux)

	paths := []string{"/health", "/health/live", "/health/ready"}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("GET %s: expected status %d, got %d", path, http.StatusOK, rr.Code)
		}
	}
}

func TestRegisterWithReadiness_EmptyConfig(t *testing.T) {
	mux := http.NewServeMux()
	RegisterWithReadiness(mux, ReadinessConfig{})

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	// Empty config = no deps to check = ready
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestRegisterWithReadiness_AllHealthy(t *testing.T) {
	mux := http.NewServeMux()
	cfg := ReadinessConfig{
		GlobalIndexDB: &mockChecker{fail: false},
		RegionalDB:    &mockChecker{fail: false},
		Redis:         &mockChecker{fail: false},
	}
	RegisterWithReadiness(mux, cfg)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if body["all_healthy"] != true {
		t.Errorf("expected all_healthy=true, got %v", body["all_healthy"])
	}

	deps, ok := body["dependencies"].([]interface{})
	if !ok || len(deps) != 3 {
		t.Fatalf("expected 3 dependencies, got %v", body["dependencies"])
	}
}

func TestRegisterWithReadiness_OneFailing(t *testing.T) {
	mux := http.NewServeMux()
	cfg := ReadinessConfig{
		GlobalIndexDB: &mockChecker{fail: false},
		RegionalDB:    &mockChecker{fail: true}, // This one fails
		Redis:         &mockChecker{fail: false},
	}
	RegisterWithReadiness(mux, cfg)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if body["all_healthy"] != false {
		t.Errorf("expected all_healthy=false, got %v", body["all_healthy"])
	}
}

func TestHandleHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	handleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type %q, got %q", "application/json", ct)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status %q, got %q", "ok", body["status"])
	}
	if body["service"] != "global-sync-service" {
		t.Errorf("expected service %q, got %q", "global-sync-service", body["service"])
	}

	ts, ok := body["timestamp"].(string)
	if !ok {
		t.Fatal("expected timestamp to be a string")
	}
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Errorf("expected timestamp in RFC3339 format, got %q: %v", ts, err)
	}
}

func TestHandleLiveness(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rr := httptest.NewRecorder()

	handleLiveness(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type %q, got %q", "application/json", ct)
	}

	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if body["status"] != "alive" {
		t.Errorf("expected status %q, got %q", "alive", body["status"])
	}
}

func TestHandleReadiness_NoDeps(t *testing.T) {
	handler := handleReadiness(ReadinessConfig{})
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if body["status"] != "ready" {
		t.Errorf("expected status %q, got %q", "ready", body["status"])
	}
}

func TestHandleReadiness_WithDepsAllHealthy(t *testing.T) {
	handler := handleReadiness(ReadinessConfig{
		GlobalIndexDB: &mockChecker{fail: false},
		Redis:         &mockChecker{fail: false},
	})
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if body["all_healthy"] != true {
		t.Errorf("expected all_healthy=true, got %v", body["all_healthy"])
	}
}

func TestHandleReadiness_WithDepsOneFailing(t *testing.T) {
	handler := handleReadiness(ReadinessConfig{
		GlobalIndexDB: &mockChecker{fail: false},
		Redis:         &mockChecker{fail: true}, // Redis fails
	})
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if body["all_healthy"] != false {
		t.Errorf("expected all_healthy=false, got %v", body["all_healthy"])
	}
}
