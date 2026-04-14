// Package consumer implements the RocketMQ message consumer for sync events.
package consumer

import (
	"context"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/service"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

// SyncConsumer processes sync events from RocketMQ.
type SyncConsumer struct {
	eventLog    *service.SyncEventLogService
	gdprChecker *service.GDPRChecker
	indexSvc    *service.GlobalIndexService
	auditSvc    *service.AuditLogService
	log         *logger.Logger
}

// NewSyncConsumer creates a new sync event consumer.
func NewSyncConsumer(
	eventLog *service.SyncEventLogService,
	gdprChecker *service.GDPRChecker,
	indexSvc *service.GlobalIndexService,
	auditSvc *service.AuditLogService,
	log *logger.Logger,
) *SyncConsumer {
	return &SyncConsumer{
		eventLog:    eventLog,
		gdprChecker: gdprChecker,
		indexSvc:    indexSvc,
		auditSvc:    auditSvc,
		log:         log,
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
		return c.indexSvc.InsertPost(ctx, event)

	case model.EventTypePostUpdated:
		return c.indexSvc.UpdatePost(ctx, event)

	case model.EventTypePostDeleted:
		return c.indexSvc.DeletePost(ctx, event)

	default:
		c.log.Warn("Unknown event type, skipping",
			logger.String("event_type", string(event.EventType)),
			logger.String("event_id", event.EventID))
		return nil
	}
}
