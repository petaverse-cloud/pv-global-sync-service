// Package service contains business logic for the Global Sync Service.
package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/postgres"
	redispkg "github.com/petaverse-cloud/pv-global-sync-service/pkg/redis"
)

// SyncEventLogService tracks processed events for idempotency.
type SyncEventLogService struct {
	db    *postgres.Manager
	redis *redispkg.Client
	log   *logger.Logger
}

// NewSyncEventLogService creates a new event log service.
func NewSyncEventLogService(db *postgres.Manager, redis *redispkg.Client, log *logger.Logger) *SyncEventLogService {
	return &SyncEventLogService{db: db, redis: redis, log: log}
}

// IsProcessed checks if an event has already been processed (Redis + DB check).
func (s *SyncEventLogService) IsProcessed(ctx context.Context, eventID string) (bool, error) {
	// Fast path: Redis set
	processed, err := s.redis.IsEventProcessed(ctx, eventID)
	if err == nil && processed {
		return true, nil
	}

	// Slow path: DB check
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM sync_event_log WHERE event_id = $1 AND status = 'processed')`
	if err := s.db.GlobalIndex().QueryRow(ctx, query, eventID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check event log: %w", err)
	}

	// Re-populate cache
	if exists {
		s.redis.MarkEventProcessed(ctx, eventID) //nolint:errcheck
	}

	return exists, nil
}

// MarkProcessed records that an event has been processed.
func (s *SyncEventLogService) MarkProcessed(ctx context.Context, event *model.CrossRegionSyncEvent, errMsg string) error {
	status := "processed"
	if errMsg != "" {
		status = "failed"
	}

	query := `
		INSERT INTO sync_event_log (event_id, event_type, source_region, status, error_message)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (event_id) DO UPDATE SET status = EXCLUDED.status, error_message = EXCLUDED.error_message
	`

	_, err := s.db.GlobalIndex().Exec(ctx, query,
		event.EventID, event.EventType, event.SourceRegion, status, errMsg,
	)
	if err != nil {
		return fmt.Errorf("write event log: %w", err)
	}

	// Update Redis cache
	if status == "processed" {
		return s.redis.MarkEventProcessed(ctx, event.EventID)
	}
	return nil
}

// ParseEvent deserializes a raw message body into a CrossRegionSyncEvent.
func ParseEvent(body []byte) (*model.CrossRegionSyncEvent, error) {
	var event model.CrossRegionSyncEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("parse sync event: %w", err)
	}

	if event.EventID == "" {
		return nil, fmt.Errorf("missing eventId in sync event")
	}
	if event.EventType == "" {
		return nil, fmt.Errorf("missing eventType in sync event")
	}
	if event.Payload.PostID == 0 {
		return nil, fmt.Errorf("missing postId in sync event payload")
	}

	return &event, nil
}
