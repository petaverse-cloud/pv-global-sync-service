// Package service contains business logic for the Global Sync Service.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/postgres"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/redis"
)

// GDPRChecker validates whether a sync event complies with GDPR rules.
//
// Rules:
//   - TIER_1 (PII): Never sync
//   - TIER_2 (UGC): Sync only if visibility == GLOBAL and user has consent
//   - TIER_3 (System): Always sync (config data, tags, places)
//   - TIER_4 (Media): Sync only if publicly accessible
//
// Visibility:
//   - GLOBAL: Eligible for cross-region sync
//   - REGIONAL: Stay in regional DB only
//   - FOLLOWERS: Stay in regional DB only
//   - PRIVATE: Never sync
type GDPRChecker struct {
	db      *postgres.Manager
	redis   *redis.Client
	log     *logger.Logger
	auditSvc *AuditLogService
}

// NewGDPRChecker creates a new GDPR compliance checker.
func NewGDPRChecker(db *postgres.Manager, redis *redis.Client, auditSvc *AuditLogService, log *logger.Logger) *GDPRChecker {
	return &GDPRChecker{db: db, redis: redis, log: log, auditSvc: auditSvc}
}

// CheckResult holds the outcome of a GDPR compliance check.
type CheckResult struct {
	Allowed bool
	Reason  string
}

// Allowed results when the event may proceed to sync.
var (
	AllowedGlobal     = CheckResult{Allowed: true, Reason: "public content with consent"}
	AllowedSystemData = CheckResult{Allowed: true, Reason: "system data - always allowed"}
)

// Denied results when the event must be rejected.
var (
	DeniedPII         = CheckResult{Allowed: false, Reason: "PII data (TIER_1) - never sync"}
	DeniedPrivate     = CheckResult{Allowed: false, Reason: "private content - cross-region prohibited"}
	DeniedFollowers   = CheckResult{Allowed: false, Reason: "followers-only content - not global"}
	DeniedRegional    = CheckResult{Allowed: false, Reason: "regional-only content - not global"}
	DeniedNoConsent   = CheckResult{Allowed: false, Reason: "user has not consented to cross-border transfer"}
	DeniedMedia       = CheckResult{Allowed: false, Reason: "media not CDN-ready for cross-region"}
)

// Check evaluates whether a sync event complies with GDPR rules.
func (c *GDPRChecker) Check(event *model.CrossRegionSyncEvent) CheckResult {
	// Rule 1: PII data is never synced
	if event.Metadata.DataCategory == model.DataCategoryPII {
		c.logSyncDecision(event, DeniedPII)
		return DeniedPII
	}

	// Rule 2: System data is always synced
	if event.Metadata.DataCategory == model.DataCategorySystem {
		c.logSyncDecision(event, AllowedSystemData)
		return AllowedSystemData
	}

	// Rule 3: Check visibility
	switch event.Payload.Visibility {
	case model.VisibilityPrivate:
		c.logSyncDecision(event, DeniedPrivate)
		return DeniedPrivate
	case model.VisibilityFollowers:
		c.logSyncDecision(event, DeniedFollowers)
		return DeniedFollowers
	case model.VisibilityRegional:
		c.logSyncDecision(event, DeniedRegional)
		return DeniedRegional
	case model.VisibilityGlobal:
		// Continue checks below
	default:
		c.logSyncDecision(event, DeniedPrivate)
		return DeniedPrivate
	}

	// Rule 4: User must have consented to cross-border transfer
	if !event.Metadata.UserConsent {
		c.logSyncDecision(event, DeniedNoConsent)
		return DeniedNoConsent
	}

	// Rule 5: Media must be CDN-ready for TIER_4
	if event.Metadata.DataCategory == model.DataCategoryMedia {
		if len(event.Payload.MediaURLs) == 0 {
			c.logSyncDecision(event, DeniedMedia)
			return DeniedMedia
		}
	}

	c.logSyncDecision(event, AllowedGlobal)
	return AllowedGlobal
}

// CheckUserConsent verifies user consent from the regional database.
// This is a fallback when the event metadata doesn't include consent info.
func (c *GDPRChecker) CheckUserConsent(ctx context.Context, userID int64) (bool, error) {
	// Try cache first
	cacheKey := fmt.Sprintf("user:consent:%d", userID)
	cached, err := c.redis.Rdb().Get(ctx, cacheKey).Result()
	if err == nil {
		return cached == "true", nil
	}

	// Query regional DB
	var consent bool
	query := `SELECT COALESCE(cross_border_transfer_allowed, false) FROM users WHERE user_id = $1`
	row := c.db.RegionalDB().QueryRow(ctx, query, userID)
	if err := row.Scan(&consent); err != nil {
		return false, fmt.Errorf("query consent for user %d: %w", userID, err)
	}

	// Cache for 5 minutes
	c.redis.Rdb().Set(ctx, cacheKey, fmt.Sprintf("%t", consent), 5*time.Minute)

	return consent, nil
}

func (c *GDPRChecker) logSyncDecision(event *model.CrossRegionSyncEvent, result CheckResult) {
	msg := "GDPR check passed"
	if !result.Allowed {
		msg = "GDPR check denied"
	}
	c.log.Infow(msg,
		"event_id", event.EventID,
		"event_type", event.EventType,
		"data_category", event.Metadata.DataCategory,
		"visibility", event.Payload.Visibility,
		"allowed", result.Allowed,
		"reason", result.Reason,
		"post_id", event.Payload.PostID,
		"author_id", event.Payload.AuthorID,
	)
}

// AuditLogService records all cross-border data transfer decisions.
type AuditLogService struct {
	db *postgres.Manager
}

// NewAuditLogService creates a new audit log service.
func NewAuditLogService(db *postgres.Manager) *AuditLogService {
	return &AuditLogService{db: db}
}

// Log records a cross-border transfer audit entry.
func (a *AuditLogService) Log(ctx context.Context, event *model.CrossRegionSyncEvent, allowed bool, reason string) error {
	status := "allowed"
	if !allowed {
		status = "denied"
	}

	query := `
		INSERT INTO cross_border_audit_log (
			event_id, data_subject_id, source_region, target_region,
			data_type, legal_basis, user_consent, status, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	metadata := fmt.Sprintf(`{"reason":%q,"visibility":%q}`, reason, event.Payload.Visibility)

	_, err := a.db.GlobalIndex().Exec(ctx, query,
		event.EventID,
		event.Payload.AuthorID,
		event.SourceRegion,
		event.TargetRegion,
		event.Metadata.DataCategory,
		"SCCs", // Standard Contractual Clauses
		event.Metadata.UserConsent,
		status,
		metadata,
	)

	return err
}
