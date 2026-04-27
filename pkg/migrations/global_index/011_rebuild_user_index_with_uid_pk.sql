-- Migration 011: Rebuild users_global_index with uid (Snowflake) as PRIMARY KEY
--
-- Why:
--   email_hash is not a suitable PK -- Google/Apple OAuth generates different hashes
--   for the same user, uid (Snowflake slug) is the true globally unique identifier.
--   This migration drops the old table and creates a clean design:
--     PK: uid (Snowflake, globally unique)
--     email_hash: optional, for pre-login check-exists lookup only
--   user_id, author_slug, author_nickname, author_avatar_url are removed:
--     user_id is regional and never queried; author_* belongs in post index not user index.
--
-- Data will be rebuilt from regional DBs after deployment.

DROP TABLE IF EXISTS users_global_index;

CREATE TABLE users_global_index (
    uid         BIGINT PRIMARY KEY,
    region      VARCHAR(16) NOT NULL,
    email_hash  VARCHAR(64),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ugi_email_hash ON users_global_index(email_hash);
CREATE INDEX IF NOT EXISTS idx_ugi_region ON users_global_index(region);
