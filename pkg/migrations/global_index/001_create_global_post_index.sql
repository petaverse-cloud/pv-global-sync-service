-- Migration 001: Global Post Index table
-- Stores cross-region post metadata (no PII, no binary media)
CREATE TABLE IF NOT EXISTS global_post_index (
    post_id BIGINT PRIMARY KEY,
    author_id BIGINT NOT NULL,
    author_region VARCHAR(16) NOT NULL,
    content_preview TEXT,
    visibility VARCHAR(20) NOT NULL,
    hashtags TEXT[],
    mentions BIGINT[],
    media_urls TEXT[],

    -- Stats (real-time sync)
    likes_count INTEGER DEFAULT 0,
    comments_count INTEGER DEFAULT 0,
    shares_count INTEGER DEFAULT 0,
    views_count INTEGER DEFAULT 0,

    -- Compliance
    gdpr_compliant BOOLEAN NOT NULL DEFAULT false,
    user_consent BOOLEAN NOT NULL DEFAULT false,
    data_category VARCHAR(20) NOT NULL,
    cross_border_ok BOOLEAN NOT NULL DEFAULT true,

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
