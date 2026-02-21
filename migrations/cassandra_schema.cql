-- Keyspace
CREATE KEYSPACE IF NOT EXISTS geoloc 
WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};

USE geoloc;

-- Users table (with auth and tracking)
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    username TEXT,
    email TEXT,
    full_name TEXT,
    bio TEXT,
    phone_number TEXT,
    profile_picture_url TEXT,
    password_hash TEXT,
    last_online TIMESTAMP,
    last_ip_address TEXT,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS ON users (username);
CREATE INDEX IF NOT EXISTS ON users (email);

-- Posts by geohash (for proximity queries)
-- Partition by geohash prefix (5-char = ~5km precision)
CREATE TABLE IF NOT EXISTS posts_by_geohash (
    geohash_prefix TEXT,
    created_at TIMESTAMP,
    post_id UUID,
    user_id UUID,
    content TEXT,
    media_urls LIST<TEXT>,
    latitude DOUBLE,
    longitude DOUBLE,
    full_geohash TEXT,
    ip_address TEXT,
    user_agent TEXT,
    PRIMARY KEY ((geohash_prefix), created_at, post_id)
) WITH CLUSTERING ORDER BY (created_at DESC, post_id ASC);

-- Posts by ID (for direct lookups)
CREATE TABLE IF NOT EXISTS posts_by_id (
    post_id UUID PRIMARY KEY,
    user_id UUID,
    content TEXT,
    media_urls LIST<TEXT>,
    latitude DOUBLE,
    longitude DOUBLE,
    geohash TEXT,
    ip_address TEXT,
    user_agent TEXT,
    created_at TIMESTAMP
);

-- Posts by user (for user profiles)
CREATE TABLE IF NOT EXISTS posts_by_user (
    user_id UUID,
    created_at TIMESTAMP,
    post_id UUID,
    content TEXT,
    media_urls LIST<TEXT>,
    latitude DOUBLE,
    longitude DOUBLE,
    ip_address TEXT,
    user_agent TEXT,
    PRIMARY KEY ((user_id), created_at, post_id)
) WITH CLUSTERING ORDER BY (created_at DESC, post_id ASC);

-- ============== LIKES ==============
-- Likes table (for posts and comments)
CREATE TABLE IF NOT EXISTS likes (
    target_type TEXT,          -- 'post' or 'comment'
    target_id UUID,            -- post_id or comment_id
    user_id UUID,
    created_at TIMESTAMP,
    PRIMARY KEY ((target_type, target_id), user_id)
);

-- Like counts (counter table for efficient counting)
CREATE TABLE IF NOT EXISTS like_counts (
    target_type TEXT,
    target_id UUID,
    count COUNTER,
    PRIMARY KEY ((target_type, target_id))
);

-- ============== COMMENTS ==============
-- Comments table (supports 3 levels of nesting)
CREATE TABLE IF NOT EXISTS comments (
    post_id UUID,              -- Root post (for all levels)
    comment_id UUID,
    parent_id UUID,            -- Parent comment ID (null for top-level)
    user_id UUID,
    content TEXT,
    depth INT,                 -- 1, 2, or 3 (max nesting level)
    ip_address TEXT,
    user_agent TEXT,
    created_at TIMESTAMP,
    PRIMARY KEY ((post_id), created_at, comment_id)
) WITH CLUSTERING ORDER BY (created_at DESC, comment_id ASC);

-- Comments by ID (for direct lookups)
CREATE TABLE IF NOT EXISTS comments_by_id (
    comment_id UUID PRIMARY KEY,
    post_id UUID,
    parent_id UUID,
    user_id UUID,
    content TEXT,
    depth INT,
    ip_address TEXT,
    user_agent TEXT,
    created_at TIMESTAMP
);

-- Comment counts per post
CREATE TABLE IF NOT EXISTS comment_counts (
    post_id UUID PRIMARY KEY,
    count COUNTER
);

-- ============== FOLLOWS ==============
-- Who a user is following
CREATE TABLE IF NOT EXISTS follows (
    follower_id UUID,
    following_id UUID,
    created_at TIMESTAMP,
    PRIMARY KEY ((follower_id), following_id)
);

-- Who follows a user (reverse lookup)
CREATE TABLE IF NOT EXISTS followers (
    user_id UUID,
    follower_id UUID,
    created_at TIMESTAMP,
    PRIMARY KEY ((user_id), created_at, follower_id)
) WITH CLUSTERING ORDER BY (created_at DESC, follower_id ASC);

-- Follow counts (counter table)
CREATE TABLE IF NOT EXISTS follow_counts (
    user_id UUID PRIMARY KEY,
    followers_count COUNTER,
    following_count COUNTER
);

-- ============== LOCATION FOLLOWS ==============
-- Users subscribing to geographic areas
CREATE TABLE IF NOT EXISTS location_follows (
    user_id UUID,
    geohash_prefix TEXT,
    name TEXT,
    latitude DOUBLE,
    longitude DOUBLE,
    created_at TIMESTAMP,
    PRIMARY KEY ((user_id), geohash_prefix)
);

-- ============== NOTIFICATIONS ==============
CREATE TABLE IF NOT EXISTS notifications (
    user_id UUID,
    notification_id UUID,
    type TEXT,
    actor_id UUID,
    target_type TEXT,
    target_id UUID,
    message TEXT,
    is_read BOOLEAN,
    created_at TIMESTAMP,
    PRIMARY KEY ((user_id), created_at, notification_id)
) WITH CLUSTERING ORDER BY (created_at DESC, notification_id ASC);

-- ============== LIKES (V2) ==============
CREATE TABLE IF NOT EXISTS like_state (
    target_type TEXT,          -- 'post' or 'comment'
    target_id UUID,            -- post_id or comment_id
    user_id UUID,
    created_at TIMESTAMP,
    PRIMARY KEY ((target_type, target_id), user_id)
);

CREATE TABLE IF NOT EXISTS likes_by_user (
    user_id UUID,
    target_type TEXT,
    target_id UUID,
    created_at TIMESTAMP,
    PRIMARY KEY ((user_id), created_at, target_id)
) WITH CLUSTERING ORDER BY (created_at DESC, target_id ASC);
