-- Migration 013: Remove legacy post_id column from global_post_index.
-- All lookups now use post_slug (globally unique Snowflake uid).
-- post_slug already exists from migration 009 and is NOT NULL.
--
-- Safety: We COALESCE scan to post_slug everywhere, so dropping post_id is safe.
ALTER TABLE global_post_index DROP COLUMN IF EXISTS post_id;

-- Rename author_id to author_uid for naming consistency
ALTER TABLE global_post_index RENAME COLUMN author_id TO author_uid;

-- Drop index on old column name, create on new
DROP INDEX IF EXISTS idx_gpi_author;
CREATE INDEX IF NOT EXISTS idx_gpi_author_uid ON global_post_index(author_uid);
