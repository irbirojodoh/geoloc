-- Remove foreign key constraint from posts table
ALTER TABLE posts DROP CONSTRAINT IF EXISTS fk_posts_user_id;

-- Drop indexes
DROP INDEX IF EXISTS idx_users_email;
DROP INDEX IF EXISTS idx_users_username;

-- Drop users table
DROP TABLE IF EXISTS users;
