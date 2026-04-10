// Package postgres provides connection pool management for PostgreSQL databases.
// The Global Sync Service uses two PostgreSQL instances:
// - Regional DB: local region's data (read-only for sync context)
// - Global Index DB: global content index (read-write)
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config holds PostgreSQL connection configuration
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// DSN returns the PostgreSQL connection string
func (c *Config) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.DBName, c.SSLMode,
	)
}

// Manager manages multiple PostgreSQL connection pools
type Manager struct {
	regionalDB *pgxpool.Pool
	globalIdx  *pgxpool.Pool
}

// NewManager creates a new PostgreSQL connection manager
func NewManager(ctx context.Context, regionalCfg, globalIdxCfg Config) (*Manager, error) {
	regionalPool, err := connect(ctx, regionalCfg, "regional")
	if err != nil {
		return nil, fmt.Errorf("regional db: %w", err)
	}

	globalIdxPool, err := connect(ctx, globalIdxCfg, "global-index")
	if err != nil {
		regionalPool.Close()
		return nil, fmt.Errorf("global index db: %w", err)
	}

	return &Manager{
		regionalDB: regionalPool,
		globalIdx:  globalIdxPool,
	}, nil
}

// RegionalDB returns the regional database connection pool
func (m *Manager) RegionalDB() *pgxpool.Pool {
	return m.regionalDB
}

// GlobalIndex returns the global index database connection pool
func (m *Manager) GlobalIndex() *pgxpool.Pool {
	return m.globalIdx
}

// Ping checks connectivity for both databases
func (m *Manager) Ping(ctx context.Context) error {
	if err := m.regionalDB.Ping(ctx); err != nil {
		return fmt.Errorf("regional db ping failed: %w", err)
	}
	if err := m.globalIdx.Ping(ctx); err != nil {
		return fmt.Errorf("global index db ping failed: %w", err)
	}
	return nil
}

// Close closes all connection pools
func (m *Manager) Close() {
	m.regionalDB.Close()
	m.globalIdx.Close()
}

func connect(ctx context.Context, cfg Config, name string) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	poolCfg.MaxConns = 25
	poolCfg.MinConns = 5
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping %s: %w", name, err)
	}

	return pool, nil
}
