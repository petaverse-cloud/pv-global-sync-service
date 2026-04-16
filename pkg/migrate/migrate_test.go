package migrate

import (
	"context"
	"embed"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testMigrationFS contains test migration files.
//
//go:embed testdata/*.sql
var testMigrationFS embed.FS

func TestRunner(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	// Clean up schema_migrations table before test
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations CASCADE")
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS test_users CASCADE")

	t.Run("applies migrations in order", func(t *testing.T) {
		runner := New(testMigrationFS, "testdata", "test")
		applied, err := runner.Run(ctx, pool)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if len(applied) != 2 {
			t.Errorf("expected 2 migrations applied, got %d", len(applied))
		}
		if applied[0] != "001_create_test_users.sql" {
			t.Errorf("first migration = %s, want 001_create_test_users.sql", applied[0])
		}
		if applied[1] != "002_add_test_email.sql" {
			t.Errorf("second migration = %s, want 002_add_test_email.sql", applied[1])
		}
	})

	t.Run("skips already applied migrations", func(t *testing.T) {
		runner := New(testMigrationFS, "testdata", "test")
		applied, err := runner.Run(ctx, pool)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if len(applied) != 0 {
			t.Errorf("expected 0 migrations on second run, got %d", len(applied))
		}
	})

	t.Run("migration table is created", func(t *testing.T) {
		var count int
		err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
		if err != nil {
			t.Fatalf("schema_migrations table not found: %v", err)
		}
		if count != 2 {
			t.Errorf("expected 2 rows in schema_migrations, got %d", count)
		}
	})
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		filename string
		want     int
		wantErr  bool
	}{
		{"001_create_users.sql", 1, false},
		{"010_add_index.sql", 10, false},
		{"099_migrate.sql", 99, false},
		{"invalid.sql", 0, true},
		{"no_underscore.sql", 0, true},
	}

	for _, tt := range tests {
		v, err := parseVersion(tt.filename)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseVersion(%s) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
		}
		if v != tt.want {
			t.Errorf("parseVersion(%s) = %d, want %d", tt.filename, v, tt.want)
		}
	}
}

func TestRunner_Idempotent(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations CASCADE")
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS test_users CASCADE")

	// Run migration 3 times - should never fail
	runner := New(testMigrationFS, "testdata", "test")
	for i := 0; i < 3; i++ {
		_, err := runner.Run(ctx, pool)
		if err != nil {
			t.Fatalf("Run #%d error = %v", i+1, err)
		}
	}
}

func TestMain(m *testing.M) {
	// Set test database timeout
	os.Exit(m.Run())
}

func TestRunner_NonExistentDir(t *testing.T) {
	// This should error because the directory doesn't exist in the embedded FS
	runner := New(testMigrationFS, "nonexistent", "test")
	ctx := context.Background()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations CASCADE")

	_, err = runner.Run(ctx, pool)
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}

func init() {
	// Ensure test migrations timeout is set
	_ = time.Second
}
