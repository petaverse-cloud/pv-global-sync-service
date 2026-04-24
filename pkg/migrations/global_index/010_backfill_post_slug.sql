-- Migration 010: Backfill post_slug for existing posts
-- Fixes the NULL post_slug issue after migration 009

-- Update existing NULL values to 0
UPDATE global_post_index SET post_slug = COALESCE(post_slug, 0);

-- Ensure NOT NULL constraint
ALTER TABLE global_post_index ALTER COLUMN post_slug SET NOT NULL;
