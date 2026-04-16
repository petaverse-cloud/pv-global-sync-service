-- Migration 004: Sync Event Log (idempotency guarantee for cross-sync)
CREATE TABLE IF NOT EXISTS sync_event_log (
    event_id VARCHAR(64) PRIMARY KEY,
    event_type VARCHAR(32) NOT NULL,
    source_region VARCHAR(16) NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_event_status ON sync_event_log(status);
CREATE INDEX IF NOT EXISTS idx_event_processed ON sync_event_log(processed_at);
