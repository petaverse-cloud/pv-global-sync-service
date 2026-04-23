-- Migration 008: Add slug to users_global_index for profile lookup
-- Currently, we can only lookup by email_hash. To support "Click Avatar -> View Profile",
-- the App sends the user's slug. We need to find the region by slug to proxy the request.
ALTER TABLE users_global_index 
  ADD COLUMN IF NOT EXISTS slug BIGINT;

CREATE INDEX IF NOT EXISTS idx_ugi_slug ON users_global_index(slug);
