package health

import (
	"encoding/json"
	"net/http"
	"time"
)

// Register registers health check endpoints on the given mux
func Register(mux *http.ServeMux) {
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/health/live", handleLiveness)
	mux.HandleFunc("/health/ready", handleReadiness)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"service":   "global-sync-service",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleLiveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "alive"})
}

func handleReadiness(w http.ResponseWriter, r *http.Request) {
	// TODO: Check database connectivity
	// TODO: Check Redis connectivity
	// TODO: Check RocketMQ connectivity

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}
