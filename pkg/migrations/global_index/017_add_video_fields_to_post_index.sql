-- 017_add_video_fields_to_post_index.sql
-- Add video URL, video cover URL, and post type columns to global_post_index
-- These fields are needed for proper feed rendering of video posts

ALTER TABLE global_post_index ADD COLUMN IF NOT EXISTS post_type INTEGER DEFAULT 1;
ALTER TABLE global_post_index ADD COLUMN IF NOT EXISTS video_url TEXT;
ALTER TABLE global_post_index ADD COLUMN IF NOT EXISTS video_cover_url TEXT;

COMMENT ON COLUMN global_post_index.post_type IS 'Post type: 1=text, 2=image, 3=video';
COMMENT ON COLUMN global_post_index.video_url IS 'Primary video URL for video posts (postType=3)';
COMMENT ON COLUMN global_post_index.video_cover_url IS 'Cover image URL for video posts';
