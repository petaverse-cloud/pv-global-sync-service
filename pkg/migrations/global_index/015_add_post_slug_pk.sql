-- Migration 015: Add PRIMARY KEY on post_slug after dropping post_id (013)
-- ON CONFLICT (post_slug) requires a unique constraint to work.
-- Without this, UPSERT will fail with: there is no unique or exclusion constraint matching the ON CONFLICT specification
ALTER TABLE global_post_index ADD PRIMARY KEY (post_slug);
