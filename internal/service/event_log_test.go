package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"

	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

// mockRedis implements EventLogRedis for testing
type mockEventLogRedis struct {
	processed map[string]bool
	failGet   bool
	failSet   bool
}

func (m *mockEventLogRedis) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	if m.failGet {
		return false, errors.New("redis error")
	}
	return m.processed[eventID], nil
}

func (m *mockEventLogRedis) MarkEventProcessed(ctx context.Context, eventID string) error {
	if m.failSet {
		return errors.New("redis error")
	}
	m.processed[eventID] = true
	return nil
}

func newMockRedis() *mockEventLogRedis {
	return &mockEventLogRedis{processed: make(map[string]bool)}
}

// ============================================
// IsProcessed tests
// ============================================

func TestIsProcessed_RedisHit(t *testing.T) {
	redis := newMockRedis()
	redis.processed["evt_001"] = true

	svc := NewSyncEventLogServiceForTest(nil, redis, logger.NewNop())

	processed, err := svc.IsProcessed(context.Background(), "evt_001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !processed {
		t.Error("expected processed=true")
	}
}

func TestIsProcessed_RedisMiss_DBHit(t *testing.T) {
	mockDB, _ := pgxmock.NewPool()
	defer mockDB.Close()

	redis := newMockRedis()
	svc := NewSyncEventLogServiceForTest(mockDB, redis, logger.NewNop())

	// Redis returns false (not found)
	// DB returns true (found)
	rows := pgxmock.NewRows([]string{"exists"}).AddRow(true)
	mockDB.ExpectQuery("SELECT EXISTS").WithArgs("evt_002").WillReturnRows(rows)

	processed, err := svc.IsProcessed(context.Background(), "evt_002")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !processed {
		t.Error("expected processed=true from DB")
	}
	// Should have re-populated Redis cache
	if !redis.processed["evt_002"] {
		t.Error("expected Redis cache to be re-populated")
	}
	if err := mockDB.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestIsProcessed_RedisMiss_DBMiss(t *testing.T) {
	mockDB, _ := pgxmock.NewPool()
	defer mockDB.Close()

	redis := newMockRedis()
	svc := NewSyncEventLogServiceForTest(mockDB, redis, logger.NewNop())

	rows := pgxmock.NewRows([]string{"exists"}).AddRow(false)
	mockDB.ExpectQuery("SELECT EXISTS").WithArgs("evt_new").WillReturnRows(rows)

	processed, err := svc.IsProcessed(context.Background(), "evt_new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if processed {
		t.Error("expected processed=false for new event")
	}
	if err := mockDB.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestIsProcessed_RedisError_FallsBackToDB(t *testing.T) {
	mockDB, _ := pgxmock.NewPool()
	defer mockDB.Close()

	redis := newMockRedis()
	redis.failGet = true // Redis fails
	svc := NewSyncEventLogServiceForTest(mockDB, redis, logger.NewNop())

	rows := pgxmock.NewRows([]string{"exists"}).AddRow(true)
	mockDB.ExpectQuery("SELECT EXISTS").WithArgs("evt_003").WillReturnRows(rows)

	processed, err := svc.IsProcessed(context.Background(), "evt_003")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !processed {
		t.Error("expected processed=true from DB fallback")
	}
	if err := mockDB.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestIsProcessed_DBError(t *testing.T) {
	mockDB, _ := pgxmock.NewPool()
	defer mockDB.Close()

	redis := newMockRedis()
	svc := NewSyncEventLogServiceForTest(mockDB, redis, logger.NewNop())

	mockDB.ExpectQuery("SELECT EXISTS").WithArgs("evt_db_err").WillReturnError(pgx.ErrNoRows)

	_, err := svc.IsProcessed(context.Background(), "evt_db_err")
	if err == nil {
		t.Fatal("expected error from DB")
	}
	if err := mockDB.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ============================================
// MarkProcessed tests
// ============================================

func TestMarkProcessed_Success(t *testing.T) {
	mockDB, _ := pgxmock.NewPool()
	defer mockDB.Close()

	redis := newMockRedis()
	svc := NewSyncEventLogServiceForTest(mockDB, redis, logger.NewNop())

	event := makeEvent(model.EventTypePostCreated, 100, 200, "test")

	mockDB.ExpectExec("INSERT INTO sync_event_log").
		WithArgs("evt_test_001", model.EventTypePostCreated, model.RegionSEA, "processed", "").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err := svc.MarkProcessed(context.Background(), event, "")
	if err != nil {
		t.Fatalf("MarkProcessed failed: %v", err)
	}
	if !redis.processed["evt_test_001"] {
		t.Error("expected Redis cache to be set")
	}
	if err := mockDB.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestMarkProcessed_WithError(t *testing.T) {
	mockDB, _ := pgxmock.NewPool()
	defer mockDB.Close()

	redis := newMockRedis()
	svc := NewSyncEventLogServiceForTest(mockDB, redis, logger.NewNop())

	event := makeEvent(model.EventTypePostCreated, 101, 201, "test")

	mockDB.ExpectExec("INSERT INTO sync_event_log").
		WithArgs("evt_test_001", model.EventTypePostCreated, model.RegionSEA, "failed", "some error").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err := svc.MarkProcessed(context.Background(), event, "some error")
	if err != nil {
		t.Fatalf("MarkProcessed failed: %v", err)
	}
	// Redis should NOT be set for failed events
	if redis.processed["evt_test_001"] {
		t.Error("expected Redis NOT to be set for failed event")
	}
	if err := mockDB.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestMarkProcessed_DBError(t *testing.T) {
	mockDB, _ := pgxmock.NewPool()
	defer mockDB.Close()

	redis := newMockRedis()
	svc := NewSyncEventLogServiceForTest(mockDB, redis, logger.NewNop())

	event := makeEvent(model.EventTypePostCreated, 102, 202, "test")

	mockDB.ExpectExec("INSERT INTO sync_event_log").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("db error"))

	err := svc.MarkProcessed(context.Background(), event, "")
	if err == nil {
		t.Fatal("expected error from DB")
	}
	if err := mockDB.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
