-- Migration 002: User follows relationship table (for push-mode fan-out)
CREATE TABLE IF NOT EXISTS user_follows (
    user_follow_uid BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    follower_uid BIGINT NOT NULL REFERENCES users(uid) ON DELETE CASCADE,
    following_uid BIGINT NOT NULL REFERENCES users(uid) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_follower_following UNIQUE (follower_uid, following_uid)
);

CREATE INDEX IF NOT EXISTS idx_following_uid ON user_follows(following_uid, created_at);
