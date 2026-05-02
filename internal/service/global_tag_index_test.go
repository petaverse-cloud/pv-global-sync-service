//go:build integration

package service

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

func setupTagTestDB(t *testing.T) (*GlobalTagIndexService, func()) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}

	// Ensure table exists
	_, err = pool.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS global_tag_index (
			tag_uid BIGINT PRIMARY KEY,
			name VARCHAR(50) NOT NULL,
			home_region VARCHAR(10) NOT NULL,
			category_uid BIGINT,
			post_count BIGINT DEFAULT 0,
			last_active_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		pool.Close()
		t.Fatalf("failed to create test table: %v", err)
	}

	log, _ := logger.New("error", "json")
	svc := NewGlobalTagIndexService(pool, log)

	cleanup := func() {
		pool.Exec(context.Background(), "DELETE FROM global_tag_index")
		pool.Close()
	}

	return svc, cleanup
}

func TestGlobalTagIndexService_UpsertAndGet(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := setupTagTestDB(t)
	defer cleanup()

	event := tagCreatedEvent(9000000001, "test-tag", model.RegionSEA)
	err := svc.UpsertTag(ctx, event)
	if err != nil {
		t.Fatalf("UpsertTag failed: %v", err)
	}

	tag, err := svc.GetTagByUID(ctx, 9000000001)
	if err != nil {
		t.Fatalf("GetTagByUID failed: %v", err)
	}
	if tag == nil {
		t.Fatal("tag should not be nil after upsert")
	}
	if tag.Name != "test-tag" {
		t.Errorf("Name = %q, want %q", tag.Name, "test-tag")
	}
	if tag.HomeRegion != "SEA" {
		t.Errorf("HomeRegion = %q, want SEA", tag.HomeRegion)
	}
}

func TestGlobalTagIndexService_UpsertUpdate(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := setupTagTestDB(t)
	defer cleanup()

	err := svc.UpsertTag(ctx, tagCreatedEvent(9000000002, "updated-tag", model.RegionSEA))
	if err != nil {
		t.Fatalf("UpsertTag failed: %v", err)
	}

	event := tagCreatedEvent(9000000002, "renamed-tag", model.RegionEU)
	event.EventType = model.EventTypeTagUpdated
	err = svc.UpsertTag(ctx, event)
	if err != nil {
		t.Fatalf("Update UpsertTag failed: %v", err)
	}

	tag, _ := svc.GetTagByUID(ctx, 9000000002)
	if tag.Name != "renamed-tag" {
		t.Errorf("Name = %q, want renamed-tag", tag.Name)
	}
}

func TestGlobalTagIndexService_Delete(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := setupTagTestDB(t)
	defer cleanup()

	err := svc.UpsertTag(ctx, tagCreatedEvent(9000000003, "delete-me", model.RegionSEA))
	if err != nil {
		t.Fatalf("UpsertTag failed: %v", err)
	}

	err = svc.DeleteTag(ctx, tagCreatedEvent(9000000003, "delete-me", model.RegionSEA))
	if err != nil {
		t.Fatalf("DeleteTag failed: %v", err)
	}

	tag, _ := svc.GetTagByUID(ctx, 9000000003)
	if tag != nil {
		t.Error("tag should be nil after delete")
	}
}

func TestGlobalTagIndexService_Search(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := setupTagTestDB(t)
	defer cleanup()

	_ = svc.UpsertTag(ctx, tagCreatedEvent(9000000010, "cat-life", model.RegionSEA))
	_ = svc.UpsertTag(ctx, tagCreatedEvent(9000000011, "dog-life", model.RegionEU))
	_ = svc.UpsertTag(ctx, tagCreatedEvent(9000000012, "cat-lovers", model.RegionSEA))

	tags, err := svc.SearchTags(ctx, "cat", 10)
	if err != nil {
		t.Fatalf("SearchTags failed: %v", err)
	}
	if len(tags) != 2 {
		t.Errorf("SearchTags count = %d, want 2", len(tags))
	}
}

func TestGlobalTagIndexService_Popular(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := setupTagTestDB(t)
	defer cleanup()

	_ = svc.UpsertTag(ctx, tagCreatedEvent(9000000020, "popular-a", model.RegionSEA))
	_ = svc.UpsertTag(ctx, tagCreatedEvent(9000000021, "popular-b", model.RegionEU))
	_ = svc.UpdateStats(ctx, 9000000020, 100)
	_ = svc.UpdateStats(ctx, 9000000021, 50)

	tags, err := svc.GetPopularTags(ctx, 10)
	if err != nil {
		t.Fatalf("GetPopularTags failed: %v", err)
	}
	if len(tags) < 2 {
		t.Errorf("GetPopularTags count = %d, want >= 2", len(tags))
	}
	if tags[0].PostCount < tags[1].PostCount {
		t.Error("popular tags not sorted by post_count DESC")
	}
}

func TestGlobalTagIndexService_UpdateStats(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := setupTagTestDB(t)
	defer cleanup()

	_ = svc.UpsertTag(ctx, tagCreatedEvent(9000000040, "stats-tag", model.RegionSEA))
	_ = svc.UpdateStats(ctx, 9000000040, 42)

	tag, _ := svc.GetTagByUID(ctx, 9000000040)
	if tag.PostCount != 42 {
		t.Errorf("PostCount = %d, want 42", tag.PostCount)
	}
	if tag.LastActiveAt == nil {
		t.Error("LastActiveAt should be set after UpdateStats with postCount > 0")
	}
}

func TestGlobalTagIndexService_NotFound(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := setupTagTestDB(t)
	defer cleanup()

	tag, err := svc.GetTagByUID(ctx, 9999999999)
	if err != nil {
		t.Fatalf("GetTagByUID unexpected error: %v", err)
	}
	if tag != nil {
		t.Error("tag should be nil for non-existent uid")
	}
}

// --- helpers ---

func tagCreatedEvent(uid int64, name string, region model.Region) *model.CrossRegionSyncEvent {
	return &model.CrossRegionSyncEvent{
		EventID:      "evt_tag_" + name,
		EventType:    model.EventTypeTagCreated,
		SourceRegion: region,
		Timestamp:    time.Now().Unix(),
		Payload: model.EventPayload{
			TagUID:  uid,
			TagName: name,
			PostID:  0,
			AuthorID: 0,
		},
		Metadata: model.EventMetadata{
			GDPRCompliant: true,
			UserConsent:   true,
			DataCategory:  model.DataCategorySystem,
			CrossBorderOK: true,
		},
	}
}
