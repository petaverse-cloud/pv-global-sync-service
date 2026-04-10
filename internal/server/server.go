// Package server holds the HTTP server and all its dependencies
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/config"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/consumer"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/handler"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/service"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/postgres"
	redispkg "github.com/petaverse-cloud/pv-global-sync-service/pkg/redis"
)

// Server holds the HTTP server and all its dependencies
type Server struct {
	httpServer *http.Server
	router     *chi.Mux
	cfg        *config.Config
	log        *logger.Logger

	// Infrastructure
	DB    *postgres.Manager
	Redis *redispkg.Client

	// Services
	IndexSvc      *service.GlobalIndexService
	AuditSvc      *service.AuditLogService
	EventLogSvc   *service.SyncEventLogService
	GDPRChecker   *service.GDPRChecker
	FeedGenerator *service.FeedGenerator

	// Consumers/Handlers
	SyncConsumer *consumer.SyncConsumer
	SyncHandler  *handler.SyncHandler
	FeedHandler  *handler.FeedHandler
}

// New creates a new server with all dependencies initialized
func New(cfg *config.Config, log *logger.Logger) (*Server, error) {
	ctx := context.Background()

	// --- Infrastructure ---
	log.Info("Connecting to PostgreSQL databases...")
	db, err := postgres.NewManager(ctx,
		postgres.Config{
			Host:     cfg.RegionalDBHost,
			Port:     cfg.RegionalDBPort,
			User:     cfg.RegionalDBUser,
			Password: cfg.RegionalDBPassword,
			DBName:   cfg.RegionalDBName,
			SSLMode:  cfg.RegionalDBSSLMode,
		},
		postgres.Config{
			Host:     cfg.GlobalIndexDBHost,
			Port:     cfg.GlobalIndexDBPort,
			User:     cfg.GlobalIndexDBUser,
			Password: cfg.GlobalIndexDBPassword,
			DBName:   cfg.GlobalIndexDBName,
			SSLMode:  cfg.GlobalIndexDBSSLMode,
		},
	)
	if err != nil {
		return nil, err
	}
	log.Info("PostgreSQL connected")

	log.Info("Connecting to Redis...")
	redis, err := redispkg.New(ctx, redispkg.Config{
		Host:     cfg.RedisHost,
		Port:     cfg.RedisPort,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	log.Info("Redis connected")

	// --- Services ---
	auditSvc := service.NewAuditLogService(db)
	indexSvc := service.NewGlobalIndexService(db.GlobalIndex(), log)
	eventLogSvc := service.NewSyncEventLogService(db, redis, log)
	gdprChecker := service.NewGDPRChecker(db, redis, auditSvc, log)

	feedGenerator := service.NewFeedGenerator(
		db, redis, indexSvc, log, cfg.FeedPushThreshold,
	)

	syncConsumer := consumer.NewSyncConsumer(
		eventLogSvc, gdprChecker, indexSvc, auditSvc, log,
	)

	syncHandler := handler.NewSyncHandler(
		syncConsumer, eventLogSvc, gdprChecker, indexSvc, auditSvc, feedGenerator, log,
	)

	feedHandler := handler.NewFeedHandler(feedGenerator, log)

	// --- Router ---
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	registerRoutes(r, db, redis, syncHandler, feedHandler, log)

	s := &Server{
		cfg:    cfg,
		log:    log,
		router: r,
		DB:     db,
		Redis:  redis,

		IndexSvc:      indexSvc,
		AuditSvc:      auditSvc,
		EventLogSvc:   eventLogSvc,
		GDPRChecker:   gdprChecker,
		FeedGenerator: feedGenerator,
		SyncConsumer:  syncConsumer,
		SyncHandler:   syncHandler,
		FeedHandler:   feedHandler,
		httpServer: &http.Server{
			Handler:           r,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}

	return s, nil
}

// registerRoutes sets up all HTTP routes
func registerRoutes(r *chi.Mux, db *postgres.Manager, redis *redispkg.Client, syncHandler *handler.SyncHandler, feedHandler *handler.FeedHandler, log *logger.Logger) {
	// Health checks
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		handleHealth(w, req, db, redis, log)
	})
	r.Get("/health/live", handleLiveness)
	r.Get("/health/ready", func(w http.ResponseWriter, req *http.Request) {
		handleReadiness(w, req, db, redis, log)
	})

	// Sync endpoints (Phase 2)
	r.Post("/sync/content", syncHandler.HandleSync)
	r.Post("/sync/cross-sync", syncHandler.HandleCrossSync)

	// Global index query (Phase 2)
	r.Get("/index/posts/{postId}", syncHandler.HandleGetPost)

	// Feed endpoints (Phase 3)
	r.Get("/feed/{userId}", feedHandler.HandleGetFeed)
}

// handleHealth returns overall service health including dependencies
func handleHealth(w http.ResponseWriter, r *http.Request, db *postgres.Manager, redis *redispkg.Client, log *logger.Logger) {
	status := "ok"

	if err := db.Ping(r.Context()); err != nil {
		status = "degraded"
		log.Warn("Database health check failed", logger.Error(err))
	}

	if err := redis.Ping(r.Context()); err != nil {
		status = "degraded"
		log.Warn("Redis health check failed", logger.Error(err))
	}

	response := map[string]interface{}{
		"status":    status,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"service":   "global-sync-service",
	}

	w.Header().Set("Content-Type", "application/json")
	if status == "degraded" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(response)
}

// handleLiveness is a simple liveness check
func handleLiveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "alive"})
}

// handleReadiness checks if all dependencies are ready
func handleReadiness(w http.ResponseWriter, r *http.Request, db *postgres.Manager, redis *redispkg.Client, log *logger.Logger) {
	if err := db.Ping(r.Context()); err != nil {
		log.Warn("Readiness check failed: database", logger.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not ready: database"})
		return
	}

	if err := redis.Ping(r.Context()); err != nil {
		log.Warn("Readiness check failed: redis", logger.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not ready: redis"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

// Listen starts the HTTP server
func (s *Server) Listen(addr string) error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server and all dependencies
func (s *Server) Shutdown(ctx context.Context) error {
	s.log.Info("Shutting down dependencies...")

	s.DB.Close()
	s.log.Info("PostgreSQL connections closed")

	if err := s.Redis.Close(); err != nil {
		s.log.Error("Error closing Redis", logger.Error(err))
	}
	s.log.Info("Redis connection closed")

	return s.httpServer.Shutdown(ctx)
}
