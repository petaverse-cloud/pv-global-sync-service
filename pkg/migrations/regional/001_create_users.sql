-- Migration 001: Users table (minimal schema for FeedGenerator follower count lookup)
-- Full wigowago user schema is managed by wigowago-api TypeORM migrations.
-- This provides the subset needed by global-sync-service.
CREATE TABLE IF NOT EXISTS users (
    user_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug BIGINT NOT NULL UNIQUE,
    username VARCHAR(50) UNIQUE NOT NULL,
    nickname VARCHAR(100),
    email VARCHAR(255) UNIQUE,
    phone VARCHAR(32) UNIQUE,
    gender SMALLINT DEFAULT 0,
    avatar_url VARCHAR(255),
    user_type SMALLINT DEFAULT 1,
    user_role SMALLINT DEFAULT 1,
    status SMALLINT DEFAULT 1,
    followers_count INTEGER DEFAULT 0,
    following_count INTEGER DEFAULT 0,
    posts_count INTEGER DEFAULT 0,
    favorite_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at);
