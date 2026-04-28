// Package consumer implements the RocketMQ message consumer for sync events.
package consumer

import (
	"context"
	"fmt"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/service"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

// SyncConsumer processes sync events from RocketMQ.
type SyncConsumer struct {
	eventLog      *service.SyncEventLogService
	gdprChecker   *service.GDPRChecker
	indexSvc      *service.GlobalIndexService
	auditSvc      *service.AuditLogService
	feedGenerator *service.FeedGenerator
	regionalDB    *pgxpool.Pool
	log           *logger.Logger
}

// NewSyncConsumer creates a new sync event consumer.
func NewSyncConsumer(
	eventLog *service.SyncEventLogService,
	gdprChecker *service.GDPRChecker,
	indexSvc *service.GlobalIndexService,
	auditSvc *service.AuditLogService,
	feedGenerator *service.FeedGenerator,
	regionalDB *pgxpool.Pool,
	log *logger.Logger,
) *SyncConsumer {
	return &SyncConsumer{
		eventLog:      eventLog,
		gdprChecker:   gdprChecker,
		indexSvc:      indexSvc,
		auditSvc:      auditSvc,
		feedGenerator: feedGenerator,
		regionalDB:    regionalDB,
		log:           log,
	}
}

// HandleMessage is the RocketMQ consumer callback.
// It processes a single sync event through the full pipeline:
//
//  1. Parse event from JSON
//  2. Check idempotency (skip if already processed)
//  3. GDPR compliance check
//  4. Route to appropriate handler (insert/update/delete)
//  5. Log audit trail
//  6. Mark as processed
func (c *SyncConsumer) HandleMessage(ctx context.Context, msg *primitive.MessageExt) (consumer.ConsumeResult, error) {
	c.log.Info("Received sync event",
		logger.String("msg_id", msg.MsgId),
		logger.Int("body_len", len(msg.Body)),
	)

	// Step 1: Parse event
	event, err := service.ParseEvent(msg.Body)
	if err != nil {
		c.log.Error("Failed to parse sync event",
			logger.String("msg_id", msg.MsgId),
			logger.Error(err))
		return consumer.ConsumeRetryLater, err
	}

	// Step 2: Idempotency check
	processed, err := c.eventLog.IsProcessed(ctx, event.EventID)
	if err != nil {
		c.log.Error("Failed to check event idempotency",
			logger.String("event_id", event.EventID),
			logger.Error(err))
		return consumer.ConsumeRetryLater, err
	}
	if processed {
		c.log.Debug("Event already processed, skipping",
			logger.String("event_id", event.EventID))
		return consumer.ConsumeSuccess, nil
	}

	// Step 3: GDPR compliance check
	checkResult := c.gdprChecker.Check(event)

	// Always log audit decision
	if auditErr := c.auditSvc.Log(ctx, event, checkResult.Allowed, checkResult.Reason); auditErr != nil {
		c.log.Error("Failed to write audit log",
			logger.String("event_id", event.EventID),
			logger.Error(auditErr))
		// Don't fail the event for audit log failure, but log it
	}

	// If not allowed, mark as processed and skip
	if !checkResult.Allowed {
		c.log.Info("Event denied by GDPR checker",
			logger.String("event_id", event.EventID),
			logger.String("reason", checkResult.Reason))

		if logErr := c.eventLog.MarkProcessed(ctx, event, "gdpr_denied: "+checkResult.Reason); logErr != nil {
			c.log.Error("Failed to log denied event", logger.Error(logErr))
		}
		return consumer.ConsumeSuccess, nil
	}

	// Step 4: Route to handler based on event type
	if err := c.routeEvent(ctx, event); err != nil {
		c.log.Error("Failed to process sync event",
			logger.String("event_id", event.EventID),
			logger.String("event_type", string(event.EventType)),
			logger.Error(err))

		if logErr := c.eventLog.MarkProcessed(ctx, event, err.Error()); logErr != nil {
			c.log.Error("Failed to log failed event", logger.Error(logErr))
		}
		return consumer.ConsumeRetryLater, err
	}

	// Step 5: Mark as processed
	if err := c.eventLog.MarkProcessed(ctx, event, ""); err != nil {
		c.log.Error("Failed to mark event as processed",
			logger.String("event_id", event.EventID),
			logger.Error(err))
		// Non-fatal: event was already applied to DB
	}

	c.log.Info("Sync event processed successfully",
		logger.String("event_id", event.EventID),
		logger.String("event_type", string(event.EventType)),
		logger.Int64("post_id", event.Payload.PostID))

	return consumer.ConsumeSuccess, nil
}

// routeEvent dispatches to the appropriate handler based on event type.
func (c *SyncConsumer) routeEvent(ctx context.Context, event *model.CrossRegionSyncEvent) error {
	switch event.EventType {
	case model.EventTypePostCreated:
		if err := c.indexSvc.InsertPost(ctx, event); err != nil {
			return err
		}
		// Trigger feed generation after successful insert
		if err := c.feedGenerator.HandleNewPost(ctx, event.Payload.AuthorID, event.Payload.PostID); err != nil {
			c.log.Error("Feed generation failed, but post was synced",
				logger.Int64("post_id", event.Payload.PostID),
				logger.Error(err))
		}
		return nil

	case model.EventTypePostUpdated:
		return c.indexSvc.UpdatePost(ctx, event)

	case model.EventTypePostDeleted:
		return c.indexSvc.DeletePost(ctx, event)

	case model.EventTypePostStatsUpdated:
		return c.handleStatsUpdated(ctx, event)

	default:
		c.log.Warn("Unknown event type, skipping",
			logger.String("event_type", string(event.EventType)),
			logger.String("event_id", event.EventID))
		return nil
	}
}

// handleStatsUpdated reads actual stats from Regional DB and updates Global Index.
func (c *SyncConsumer) handleStatsUpdated(ctx context.Context, event *model.CrossRegionSyncEvent) error {
	postSlug := event.Payload.PostSlug
	postID := event.Payload.PostID

	var likes, comments, favorites, views int
	// Note: Regional DB has favorites_count, not shares_count
	query := `SELECT likes_count, comments_count, favorites_count, views_count FROM posts WHERE post_id = $1`
	err := c.regionalDB.QueryRow(ctx, query, postID).Scan(&likes, &comments, &favorites, &views)
	if err != nil {
		return fmt.Errorf("read stats for post %d from regional db: %w", postID, err)
	}

	if err := c.indexSvc.UpdateStats(ctx, postSlug, likes, comments, favorites, views); err != nil {
		return fmt.Errorf("update stats for post slug=%d in global index: %w", postSlug, err)
	}

	c.log.Info("Post stats updated in global index",
		logger.Int64("post_id", postID),
		logger.Int64("post_slug", postSlug),
		logger.Int("likes", likes),
		logger.Int("comments", comments),
		logger.Int("favorites", favorites),
		logger.Int("views", views))

	return nil
}
