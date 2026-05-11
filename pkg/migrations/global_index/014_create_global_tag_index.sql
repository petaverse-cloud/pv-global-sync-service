-- Migration 014: Create global_tag_index table
-- Replicates public tag metadata across regions for cross-region tag search.
CREATE TABLE IF NOT EXISTS global_tag_index (
    tag_uid        BIGINT PRIMARY KEY,
    name           VARCHAR(50) NOT NULL,
    home_region    VARCHAR(10) NOT NULL,
    category_uid   BIGINT,
    post_count     BIGINT DEFAULT 0,
    last_active_at TIMESTAMPTZ,
    created_at     TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_global_tag_home ON global_tag_index(home_region);
CREATE INDEX IF NOT EXISTS idx_global_tag_name ON global_tag_index(name);
