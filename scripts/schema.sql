-- Global Sync Service Database Schema
-- Aligned with wigowago-v2-distributed-architecture.md

-- ============================================================
-- Global Post Index Table
-- ============================================================
CREATE TABLE IF NOT EXISTS global_post_index (
    post_id BIGINT PRIMARY KEY,
    author_id BIGINT NOT NULL,
    author_region VARCHAR(16) NOT NULL,
    content_preview TEXT,
    visibility VARCHAR(20) NOT NULL,
    hashtags TEXT[],
    mentions BIGINT[],

    -- Stats (real-time sync)
    likes_count INTEGER DEFAULT 0,
    comments_count INTEGER DEFAULT 0,
    shares_count INTEGER DEFAULT 0,
    views_count INTEGER DEFAULT 0,

    -- Compliance
    gdpr_compliant BOOLEAN NOT NULL DEFAULT false,
    user_consent BOOLEAN NOT NULL DEFAULT false,
    data_category VARCHAR(20) NOT NULL,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL,
    synced_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_gpi_author ON global_post_index(author_id);
CREATE INDEX IF NOT EXISTS idx_gpi_visibility ON global_post_index(visibility);
CREATE INDEX IF NOT EXISTS idx_gpi_hashtags ON global_post_index USING GIN(hashtags);
CREATE INDEX IF NOT EXISTS idx_gpi_created ON global_post_index(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_gpi_region ON global_post_index(author_region);

-- ============================================================
-- User Feed Table (pre-computed)
-- ============================================================
CREATE TABLE IF NOT EXISTS user_feed (
    user_id BIGINT NOT NULL,
    post_id BIGINT NOT NULL,
    feed_type VARCHAR(20) NOT NULL,  -- 'following' | 'global' | 'trending'
    score DECIMAL(10,6) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ,

    PRIMARY KEY (user_id, feed_type, post_id)
);

CREATE INDEX IF NOT EXISTS idx_uf_user ON user_feed(user_id, feed_type, score DESC);
CREATE INDEX IF NOT EXISTS idx_uf_expires ON user_feed(expires_at) WHERE expires_at IS NOT NULL;

-- ============================================================
-- Cross-Border Audit Log (GDPR compliance)
-- ============================================================
CREATE TABLE IF NOT EXISTS cross_border_audit_log (
    log_id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(64) NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    data_subject_id BIGINT NOT NULL,
    source_region VARCHAR(16) NOT NULL,
    target_region VARCHAR(16) NOT NULL,
    data_type VARCHAR(50) NOT NULL,
    legal_basis VARCHAR(100),
    user_consent BOOLEAN DEFAULT false,
    status VARCHAR(20) NOT NULL,
    metadata JSONB
);

CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON cross_border_audit_log(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_subject ON cross_border_audit_log(data_subject_id);
CREATE INDEX IF NOT EXISTS idx_audit_status ON cross_border_audit_log(status);

-- ============================================================
-- Sync Event Log (idempotency guarantee)
-- ============================================================
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

-- ============================================================
-- Cleanup old audit logs (run periodically)
-- ============================================================
-- Example: DELETE FROM cross_border_audit_log WHERE timestamp < NOW() - INTERVAL '1 year';
