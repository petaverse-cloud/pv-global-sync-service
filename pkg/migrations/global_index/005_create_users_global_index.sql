-- Migration 005: Global User Index table
-- Stores email_hash -> region mapping for cross-region user existence checks.
-- Privacy: Stores hash only, no PII.
CREATE TABLE IF NOT EXISTS users_global_index (
    email_hash VARCHAR(64) PRIMARY KEY,
    user_id BIGINT NOT NULL,
    region VARCHAR(16) NOT NULL,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ugi_region ON users_global_index(region);
