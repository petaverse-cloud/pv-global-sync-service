-- Migration 013: Remove legacy post_id and author_id columns from global_post_index.
-- All lookups now use post_slug and author_uid (already added by migrations 009/012).
--
-- Safety: We COALESCE scan to post_slug everywhere, so dropping post_id is safe.
ALTER TABLE global_post_index DROP COLUMN IF EXISTS post_id;

-- author_uid was already added by migration 012, so just drop the legacy author_id
ALTER TABLE global_post_index DROP COLUMN IF EXISTS author_id;

-- Drop index on old column name, create on new if not exists
DROP INDEX IF EXISTS idx_gpi_author;
CREATE INDEX IF NOT EXISTS idx_gpi_author_uid ON global_post_index(author_uid);
