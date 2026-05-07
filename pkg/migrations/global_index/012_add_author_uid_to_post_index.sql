-- 012_add_author_uid_to_post_index.sql
-- Rename author_id → author_uid, backfill from author_slug (Snowflake UID)
-- Must be applied BEFORE deploying Phase 1-3 Go code changes.

DO $$
BEGIN
    -- Step 1: Add author_uid column (will become the new authority column)
    ALTER TABLE global_post_index ADD COLUMN IF NOT EXISTS author_uid BIGINT;
    
    -- Step 2: Backfill author_uid from author_slug (Snowflake UID)
    UPDATE global_post_index 
    SET author_uid = author_slug 
    WHERE author_uid IS NULL AND author_slug IS NOT NULL AND author_slug > 0;
    
    -- Step 3: Set remaining NULLs to 0
    UPDATE global_post_index SET author_uid = 0 WHERE author_uid IS NULL;
    
    -- Step 4: Create index on author_uid (replacing old author_id index usage)
    CREATE INDEX IF NOT EXISTS idx_gpi_author_uid ON global_post_index(author_uid);
    
    RAISE NOTICE 'Migration 012 complete: author_uid column added and backfilled';
END $$;
