package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Checker defines health check interfaces.
type Checker interface {
	Ping(ctx context.Context) error
}

// ReadinessConfig holds dependencies for readiness probe.
type ReadinessConfig struct {
	GlobalIndexDB Checker
	RegionalDB    Checker
	Redis         Checker
}

// Register registers health check endpoints on the given mux.
func Register(mux *http.ServeMux) {
	RegisterWithReadiness(mux, ReadinessConfig{})
}

// RegisterWithReadiness registers health endpoints with optional readiness dependencies.
func RegisterWithReadiness(mux *http.ServeMux, cfg ReadinessConfig) {
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/health/live", handleLiveness)
	mux.HandleFunc("/health/ready", handleReadiness(cfg))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"service":   "global-sync-service",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func handleLiveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "alive"}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleReadiness(cfg ReadinessConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		type depCheck struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		}
		var deps []depCheck
		allReady := true

		if cfg.GlobalIndexDB != nil {
			status := "ok"
			if err := cfg.GlobalIndexDB.Ping(ctx); err != nil {
				status = "fail"
				allReady = false
			}
			deps = append(deps, depCheck{"global_index_db", status})
		}
		if cfg.RegionalDB != nil {
			status := "ok"
			if err := cfg.RegionalDB.Ping(ctx); err != nil {
				status = "fail"
				allReady = false
			}
			deps = append(deps, depCheck{"regional_db", status})
		}
		if cfg.Redis != nil {
			status := "ok"
			if err := cfg.Redis.Ping(ctx); err != nil {
				status = "fail"
				allReady = false
			}
			deps = append(deps, depCheck{"redis", status})
		}

		response := map[string]interface{}{
			"status":       "ready",
			"all_healthy":  allReady,
			"dependencies": deps,
		}

		if !allReady {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
