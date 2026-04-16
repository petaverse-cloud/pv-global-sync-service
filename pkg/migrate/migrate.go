// Package migrate provides automatic database schema migration for PostgreSQL.
//
// It embeds SQL migration files using go:embed, tracks applied migrations in a
// schema_migrations table, and runs them in order on startup. Migration files
// are named with numeric prefixes for ordering (e.g., 001_create_users.sql).
//
// Usage in server startup:
//
//	applied, err := migrate.Run(ctx, pool, migrations.GlobalIndexFS, "global_index")
package migrate

import (
	"context"
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Runner applies embedded SQL migrations to a PostgreSQL database.
type Runner struct {
	files  embed.FS
	dir    string
	prefix string
}

// New creates a new migration runner.
// fs is the embedded filesystem, dir is the subdirectory within fs containing
// the migration files, and prefix is an optional label for log messages.
func New(fs embed.FS, dir string, prefix string) *Runner {
	return &Runner{files: fs, dir: dir, prefix: prefix}
}

// Run applies all pending migrations to the given connection pool.
// It returns the list of applied migration filenames.
func (r *Runner) Run(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	// Ensure schema_migrations table exists
	if err := r.ensureMigrationTable(ctx, pool); err != nil {
		return nil, fmt.Errorf("ensure migration table: %w", err)
	}

	// Get already applied migrations
	applied, err := r.getApplied(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("get applied migrations: %w", err)
	}

	// Get all migration files
	files, err := r.getMigrationFiles()
	if err != nil {
		return nil, fmt.Errorf("list migration files: %w", err)
	}

	var appliedThisRun []string
	for _, f := range files {
		if applied[f.version] {
			continue
		}

		if err := r.applyMigration(ctx, pool, f); err != nil {
			return appliedThisRun, fmt.Errorf("apply migration %s: %w", f.name, err)
		}

		if err := r.recordApplied(ctx, pool, f.version); err != nil {
			return appliedThisRun, fmt.Errorf("record migration %s: %w", f.name, err)
		}

		appliedThisRun = append(appliedThisRun, f.name)
	}

	return appliedThisRun, nil
}

func (r *Runner) ensureMigrationTable(ctx context.Context, pool *pgxpool.Pool) error {
	query := `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`
	_, err := pool.Exec(ctx, query)
	return err
}

func (r *Runner) getApplied(ctx context.Context, pool *pgxpool.Pool) (map[int]bool, error) {
	rows, err := pool.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func (r *Runner) recordApplied(ctx context.Context, pool *pgxpool.Pool, version int) error {
	name, err := r.fileNameForVersion(version)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx,
		"INSERT INTO schema_migrations (version, name) VALUES ($1, $2) ON CONFLICT (version) DO NOTHING",
		version, name,
	)
	return err
}

type migrationFile struct {
	name    string
	version int
	content string
}

func (r *Runner) getMigrationFiles() ([]migrationFile, error) {
	entries, err := r.files.ReadDir(r.dir)
	if err != nil {
		return nil, err
	}

	var files []migrationFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}

		version, err := parseVersion(e.Name())
		if err != nil {
			continue // Skip non-migration files
		}

		content, err := r.files.ReadFile(filepath.Join(r.dir, e.Name()))
		if err != nil {
			return nil, err
		}

		files = append(files, migrationFile{
			name:    e.Name(),
			version: version,
			content: string(content),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].version < files[j].version
	})

	return files, nil
}

func (r *Runner) fileNameForVersion(version int) (string, error) {
	files, err := r.getMigrationFiles()
	if err != nil {
		return "", err
	}
	for _, f := range files {
		if f.version == version {
			return f.name, nil
		}
	}
	return "", fmt.Errorf("no file found for version %d", version)
}

func (r *Runner) applyMigration(ctx context.Context, pool *pgxpool.Pool, f migrationFile) error {
	label := f.name
	if r.prefix != "" {
		label = fmt.Sprintf("[%s] %s", r.prefix, f.name)
	}
	if _, err := pool.Exec(ctx, f.content); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

func parseVersion(filename string) (int, error) {
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid migration filename: %s", filename)
	}
	return strconv.Atoi(parts[0])
}
