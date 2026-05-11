package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

func makeTagEvent(tagUID int64, tagName string, eventType model.SyncEventType) *model.CrossRegionSyncEvent {
	catUID := int64(10)
	return &model.CrossRegionSyncEvent{
		EventID:      "evt_tag_001",
		EventType:    eventType,
		SourceRegion: model.RegionSEA,
		Payload: model.EventPayload{
			TagUID:         tagUID,
			TagName:        tagName,
			TagCategoryUID: &catUID,
		},
	}
}

// ===== UpsertTag =====

func TestTagUpsert_Insert(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalTagIndexServiceWithDB(mock, logger.NewNop())

	mock.ExpectExec("INSERT INTO global_tag_index").
		WithArgs(int64(100), "golang", model.RegionSEA, pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	if err := svc.UpsertTag(context.Background(), makeTagEvent(100, "golang", model.EventTypeTagCreated)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestTagUpsert_Update(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalTagIndexServiceWithDB(mock, logger.NewNop())

	// ON CONFLICT DO UPDATE — same SQL, upsert behavior
	mock.ExpectExec("INSERT INTO global_tag_index").
		WithArgs(int64(100), "golang", model.RegionSEA, pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	if err := svc.UpsertTag(context.Background(), makeTagEvent(100, "golang", model.EventTypeTagUpdated)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestTagUpsert_DBError(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalTagIndexServiceWithDB(mock, logger.NewNop())

	mock.ExpectExec("INSERT INTO global_tag_index").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(fmt.Errorf("connection closed"))

	if err := svc.UpsertTag(context.Background(), makeTagEvent(100, "golang", model.EventTypeTagCreated)); err == nil {
		t.Fatal("expected error")
	}
}

// ===== DeleteTag =====

func TestTagDelete_Success(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalTagIndexServiceWithDB(mock, logger.NewNop())

	mock.ExpectExec("DELETE FROM global_tag_index").
		WithArgs(int64(200)).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	if err := svc.DeleteTag(context.Background(), makeTagEvent(200, "deleteme", model.EventTypeTagDeleted)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestTagDelete_NotFound(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalTagIndexServiceWithDB(mock, logger.NewNop())

	mock.ExpectExec("DELETE FROM global_tag_index").
		WithArgs(int64(999)).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	// Delete non-existent tag should not error
	if err := svc.DeleteTag(context.Background(), makeTagEvent(999, "ghost", model.EventTypeTagDeleted)); err != nil {
		t.Fatal(err)
	}
}

// ===== UpdateStats =====

func TestTagUpdateStats_WithPosts(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalTagIndexServiceWithDB(mock, logger.NewNop())

	// postCount > 0 → last_active_at set to now
	mock.ExpectExec("UPDATE global_tag_index").
		WithArgs(int64(42), pgxmock.AnyArg(), int64(300)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	if err := svc.UpdateStats(context.Background(), 300, 42); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestTagUpdateStats_ZeroPosts(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalTagIndexServiceWithDB(mock, logger.NewNop())

	// postCount == 0 → last_active_at = nil
	mock.ExpectExec("UPDATE global_tag_index").
		WithArgs(int64(0), (*time.Time)(nil), int64(301)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	if err := svc.UpdateStats(context.Background(), 301, 0); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// ===== SearchTags =====

func TestTagSearch_Found(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalTagIndexServiceWithDB(mock, logger.NewNop())
	now := time.Now().UTC()

	rows := pgxmock.NewRows([]string{
		"tag_uid", "name", "home_region", "category_uid", "post_count",
		"last_active_at", "created_at", "updated_at",
	}).
		AddRow(int64(400), "golang", "SEA", ptrI64(10), int64(42), &now, now, now).
		AddRow(int64(401), "gopher", "EU", ptrI64(11), int64(15), &now, now, now)

	mock.ExpectQuery("SELECT tag_uid.*FROM global_tag_index.*WHERE name ILIKE").
		WithArgs("go", 20).
		WillReturnRows(rows)

	tags, err := svc.SearchTags(context.Background(), "go", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 {
		t.Fatalf("len=%d want 2", len(tags))
	}
	if tags[0].Name != "golang" {
		t.Errorf("tags[0].Name=%s", tags[0].Name)
	}
	if tags[1].Name != "gopher" {
		t.Errorf("tags[1].Name=%s", tags[1].Name)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestTagSearch_Empty(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalTagIndexServiceWithDB(mock, logger.NewNop())

	rows := pgxmock.NewRows([]string{"tag_uid", "name", "home_region", "category_uid", "post_count", "last_active_at", "created_at", "updated_at"})
	mock.ExpectQuery("SELECT").WithArgs("nonexistent", 20).WillReturnRows(rows)

	tags, err := svc.SearchTags(context.Background(), "nonexistent", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 0 {
		t.Errorf("len=%d want 0", len(tags))
	}
}

// ===== GetPopularTags =====

func TestTagGetPopular(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalTagIndexServiceWithDB(mock, logger.NewNop())
	now := time.Now().UTC()

	rows := pgxmock.NewRows([]string{
		"tag_uid", "name", "home_region", "category_uid", "post_count",
		"last_active_at", "created_at", "updated_at",
	}).AddRow(int64(500), "popular", "SEA", nil, int64(100), &now, now, now)

	mock.ExpectQuery("SELECT tag_uid.*FROM global_tag_index.*ORDER BY post_count DESC").
		WithArgs(10).
		WillReturnRows(rows)

	tags, err := svc.GetPopularTags(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 {
		t.Fatalf("len=%d", len(tags))
	}
	if tags[0].PostCount != 100 {
		t.Errorf("postCount=%d", tags[0].PostCount)
	}
}

// ===== GetTagByUID =====

func TestTagGetByUID_Found(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalTagIndexServiceWithDB(mock, logger.NewNop())
	now := time.Now().UTC()

	rows := pgxmock.NewRows([]string{
		"tag_uid", "name", "home_region", "category_uid", "post_count",
		"last_active_at", "created_at", "updated_at",
	}).AddRow(int64(600), "unique", "EU", ptrI64(20), int64(5), &now, now, now)

	mock.ExpectQuery("SELECT tag_uid.*FROM global_tag_index WHERE tag_uid").
		WithArgs(int64(600)).
		WillReturnRows(rows)

	tag, err := svc.GetTagByUID(context.Background(), 600)
	if err != nil {
		t.Fatal(err)
	}
	if tag == nil {
		t.Fatal("expected tag")
	}
	if tag.TagUID != 600 {
		t.Errorf("TagUID=%d", tag.TagUID)
	}
	if tag.Name != "unique" {
		t.Errorf("Name=%s", tag.Name)
	}
}

func TestTagGetByUID_NotFound(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalTagIndexServiceWithDB(mock, logger.NewNop())

	mock.ExpectQuery("SELECT").WithArgs(int64(999)).WillReturnError(pgx.ErrNoRows)

	tag, err := svc.GetTagByUID(context.Background(), 999)
	if err != nil {
		t.Fatal(err)
	}
	if tag != nil {
		t.Error("expected nil")
	}
}

// ===== GetRegionsForTag =====

func TestTagGetRegions(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalTagIndexServiceWithDB(mock, logger.NewNop())

	rows := pgxmock.NewRows([]string{"home_region"}).
		AddRow("SEA").AddRow("EU").AddRow("NA")
	mock.ExpectQuery("SELECT DISTINCT home_region FROM global_tag_index").
		WithArgs(int64(700)).
		WillReturnRows(rows)

	regions, err := svc.GetRegionsForTag(context.Background(), 700)
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 3 {
		t.Fatalf("len=%d", len(regions))
	}
	if regions[0] != "SEA" || regions[1] != "EU" || regions[2] != "NA" {
		t.Errorf("regions=%v", regions)
	}
}

func TestTagGetRegions_Empty(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalTagIndexServiceWithDB(mock, logger.NewNop())

	rows := pgxmock.NewRows([]string{"home_region"})
	mock.ExpectQuery("SELECT DISTINCT").WithArgs(int64(888)).WillReturnRows(rows)

	regions, err := svc.GetRegionsForTag(context.Background(), 888)
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 0 {
		t.Errorf("len=%d", len(regions))
	}
}

func ptrI64(v int64) *int64 { return &v }
