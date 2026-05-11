-- Migration 016: Rename post_slug → uid in global_post_index
-- Aligns Global Index with the uid-based PK convention used across all other domains.
-- post_slug was a legacy naming artifact from the pre-uid era.
BEGIN;

-- Step 1: Drop the primary key constraint (it's on post_slug, must be dropped before rename)
ALTER TABLE global_post_index DROP CONSTRAINT IF EXISTS global_post_index_pkey;

-- Step 2: Rename the column
ALTER TABLE global_post_index RENAME COLUMN post_slug TO uid;

-- Step 3: Rename the index (PostgreSQL doesn't auto-rename indexes on column rename)
ALTER INDEX IF EXISTS idx_gpi_post_slug RENAME TO idx_gpi_uid;

-- Step 4: Re-create primary key on uid
ALTER TABLE global_post_index ADD PRIMARY KEY (uid);

COMMIT;
