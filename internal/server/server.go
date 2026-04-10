package server

import (
	"context"
	"net/http"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/config"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/health"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

// Server holds the HTTP server and its dependencies
type Server struct {
	httpServer *http.Server
	cfg        *config.Config
	log        *logger.Logger
}

// New creates a new server instance
func New(cfg *config.Config, log *logger.Logger) (*Server, error) {
	mux := http.NewServeMux()

	// Register health check routes
	health.Register(mux)

	// TODO: Register business routes
	// - POST /sync/content - receive sync events from local API
	// - POST /sync/cross-sync - receive cross-region sync events
	// - GET /feed/generate - generate feed for a user
	// - GET /feed/:userId - get user's feed

	// TODO: Register middleware
	// - Request logging
	// - Recovery/panic handling
	// - Metrics

	s := &Server{
		cfg: cfg,
		log: log,
		httpServer: &http.Server{
			Handler: mux,
		},
	}

	return s, nil
}

// Listen starts the HTTP server
func (s *Server) Listen(addr string) error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	// TODO: Shutdown RocketMQ consumer
	// TODO: Shutdown RocketMQ producer
	// TODO: Close database connections
	// TODO: Close Redis connection
	return s.httpServer.Shutdown(ctx)
}
