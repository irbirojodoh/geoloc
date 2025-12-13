-- Drop indexes
DROP INDEX IF EXISTS idx_posts_user_id;
DROP INDEX IF EXISTS idx_posts_created_at;
DROP INDEX IF EXISTS idx_posts_location;

-- Drop posts table
DROP TABLE IF EXISTS posts;

-- Drop PostGIS extension (optional - uncomment if needed)
-- DROP EXTENSION IF EXISTS postgis;
