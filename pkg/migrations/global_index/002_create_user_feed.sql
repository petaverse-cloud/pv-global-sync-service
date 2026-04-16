-- Migration 002: User Feed table (pre-computed feed items)
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
