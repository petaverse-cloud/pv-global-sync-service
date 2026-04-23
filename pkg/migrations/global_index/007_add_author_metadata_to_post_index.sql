-- Migration 007: Add author metadata to global_post_index
-- Stores public author info for cross-region feed display to avoid querying remote DB.
ALTER TABLE global_post_index 
  ADD COLUMN IF NOT EXISTS author_slug BIGINT,
  ADD COLUMN IF NOT EXISTS author_nickname VARCHAR(100),
  ADD COLUMN IF NOT EXISTS author_avatar_url VARCHAR(255);
