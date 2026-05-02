// Package service contains business logic for the Global Sync Service.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

// GlobalTagIndexService manages operations on the global_tag_index table.
type GlobalTagIndexService struct {
	db  *pgxpool.Pool
	log *logger.Logger
}

// NewGlobalTagIndexService creates a new service instance.
func NewGlobalTagIndexService(db *pgxpool.Pool, log *logger.Logger) *GlobalTagIndexService {
	return &GlobalTagIndexService{db: db, log: log}
}

// UpsertTag inserts or updates a tag in the global_tag_index.
// Used by TAG_CREATED and TAG_UPDATED events.
func (s *GlobalTagIndexService) UpsertTag(ctx context.Context, event *model.CrossRegionSyncEvent) error {
	now := time.Now().UTC()

	query := `
		INSERT INTO global_tag_index (tag_uid, name, home_region, category_uid, post_count, last_active_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 0, NULL, $5, $5)
		ON CONFLICT (tag_uid) DO UPDATE SET
			name = EXCLUDED.name,
			home_region = EXCLUDED.home_region,
			category_uid = EXCLUDED.category_uid,
			updated_at = NOW()
	`

	_, err := s.db.Exec(ctx, query,
		event.Payload.TagUID,
		event.Payload.TagName,
		event.SourceRegion,
		event.Payload.TagCategoryUID,
		now,
	)
	if err != nil {
		return fmt.Errorf("upsert tag uid=%d name=%s: %w", event.Payload.TagUID, event.Payload.TagName, err)
	}

	s.log.Info("Tag upserted into global index",
		logger.Int64("tag_uid", event.Payload.TagUID),
		logger.String("name", event.Payload.TagName),
		logger.String("region", string(event.SourceRegion)),
	)
	return nil
}

// DeleteTag removes a tag from the global_tag_index.
// Used by TAG_DELETED events.
func (s *GlobalTagIndexService) DeleteTag(ctx context.Context, event *model.CrossRegionSyncEvent) error {
	result, err := s.db.Exec(ctx, `DELETE FROM global_tag_index WHERE tag_uid = $1`, event.Payload.TagUID)
	if err != nil {
		return fmt.Errorf("delete tag uid=%d: %w", event.Payload.TagUID, err)
	}

	s.log.Info("Tag deleted from global index",
		logger.Int64("tag_uid", event.Payload.TagUID),
		logger.Int64("rows_affected", result.RowsAffected()),
	)
	return nil
}

// UpdateTagStats updates post_count for a tag.
// Used by TAG_STATS_UPDATED events.
func (s *GlobalTagIndexService) UpdateStats(ctx context.Context, tagUID int64, postCount int64) error {
	var lastActiveAt *time.Time
	if postCount > 0 {
		now := time.Now().UTC()
		lastActiveAt = &now
	}

	_, err := s.db.Exec(ctx,
		`UPDATE global_tag_index SET post_count = $1, last_active_at = $2, updated_at = NOW() WHERE tag_uid = $3`,
		postCount, lastActiveAt, tagUID,
	)
	return err
}

// SearchTags searches tags by keyword in the global index.
func (s *GlobalTagIndexService) SearchTags(ctx context.Context, keyword string, limit int) ([]model.GlobalTagIndex, error) {
	query := `
		SELECT tag_uid, name, home_region, category_uid, post_count, last_active_at, created_at, updated_at
		FROM global_tag_index
		WHERE name ILIKE '%' || $1 || '%'
		ORDER BY post_count DESC, name ASC
		LIMIT $2
	`

	rows, err := s.db.Query(ctx, query, keyword, limit)
	if err != nil {
		return nil, fmt.Errorf("search tags keyword=%s: %w", keyword, err)
	}
	defer rows.Close()

	return scanTagRows(rows)
}

// GetPopularTags returns tags sorted by post_count descending.
func (s *GlobalTagIndexService) GetPopularTags(ctx context.Context, limit int) ([]model.GlobalTagIndex, error) {
	query := `
		SELECT tag_uid, name, home_region, category_uid, post_count, last_active_at, created_at, updated_at
		FROM global_tag_index
		ORDER BY post_count DESC, name ASC
		LIMIT $1
	`

	rows, err := s.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("get popular tags: %w", err)
	}
	defer rows.Close()

	return scanTagRows(rows)
}

// GetTagByUID retrieves a single tag by its globally unique UID.
func (s *GlobalTagIndexService) GetTagByUID(ctx context.Context, tagUID int64) (*model.GlobalTagIndex, error) {
	var tag model.GlobalTagIndex

	err := s.db.QueryRow(ctx,
		`SELECT tag_uid, name, home_region, category_uid, post_count, last_active_at, created_at, updated_at
		 FROM global_tag_index WHERE tag_uid = $1`, tagUID,
	).Scan(&tag.TagUID, &tag.Name, &tag.HomeRegion, &tag.CategoryUID, &tag.PostCount,
		&tag.LastActiveAt, &tag.CreatedAt, &tag.UpdatedAt)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tag uid=%d: %w", tagUID, err)
	}

	return &tag, nil
}

// GetRegionsForTag returns distinct regions that have a given tag.
func (s *GlobalTagIndexService) GetRegionsForTag(ctx context.Context, tagUID int64) ([]string, error) {
	rows, err := s.db.Query(ctx,
		`SELECT DISTINCT home_region FROM global_tag_index WHERE tag_uid = $1`, tagUID,
	)
	if err != nil {
		return nil, fmt.Errorf("get regions for tag uid=%d: %w", tagUID, err)
	}
	defer rows.Close()

	var regions []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		regions = append(regions, r)
	}
	return regions, rows.Err()
}

func scanTagRows(rows pgx.Rows) ([]model.GlobalTagIndex, error) {
	var tags []model.GlobalTagIndex
	for rows.Next() {
		var t model.GlobalTagIndex
		if err := rows.Scan(&t.TagUID, &t.Name, &t.HomeRegion, &t.CategoryUID,
			&t.PostCount, &t.LastActiveAt, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}
