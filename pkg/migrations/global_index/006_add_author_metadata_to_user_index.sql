-- Migration 006: Add author metadata to users_global_index
-- Stores public profile info (slug, nickname, avatar_url) for cross-region feed display.
-- This allows the feed to show correct author info without querying the remote region's DB.
ALTER TABLE users_global_index 
  ADD COLUMN IF NOT EXISTS author_slug BIGINT,
  ADD COLUMN IF NOT EXISTS author_nickname VARCHAR(100),
  ADD COLUMN IF NOT EXISTS author_avatar_url VARCHAR(255);

-- Backfill existing rows with defaults (null) is handled by ADD COLUMN IF NOT EXISTS
