-- Migration 002: User follows relationship table (for push-mode fan-out)
CREATE TABLE IF NOT EXISTS user_follows (
    user_follow_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    follower_id BIGINT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    following_id BIGINT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_follower_following UNIQUE (follower_id, following_id)
);

CREATE INDEX IF NOT EXISTS idx_following_id ON user_follows(following_id, created_at);
