-- Use NetworkTopologyStrategy for production. Using SimpleStrategy 1 as fallback for local dev.
CREATE KEYSPACE IF NOT EXISTS geoloc 
WITH replication = {'class': 'NetworkTopologyStrategy', 'datacenter1': 3} 
AND durable_writes = true;

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
    cover_image_url TEXT,
    password_hash TEXT,
    last_online TIMESTAMP,
    last_ip_address TEXT,
    is_deleted BOOLEAN,
    deleted_at TIMESTAMP,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

-- Storage-Attached Indexes (SAI) for scalable search without ALLOW FILTERING
-- normalize=true is REQUIRED for SAI LIKE (prefix/contains) matching
-- Drop first so existing indexes get the new options on re-init
DROP INDEX IF EXISTS username_sai_idx;
DROP INDEX IF EXISTS email_sai_idx;
DROP INDEX IF EXISTS fullname_sai_idx;
CREATE CUSTOM INDEX username_sai_idx ON users (username) USING 'StorageAttachedIndex' WITH OPTIONS = {'case_sensitive': 'false', 'normalize': 'true'};
CREATE CUSTOM INDEX email_sai_idx ON users (email) USING 'StorageAttachedIndex' WITH OPTIONS = {'case_sensitive': 'false', 'normalize': 'true'};
CREATE CUSTOM INDEX fullname_sai_idx ON users (full_name) USING 'StorageAttachedIndex' WITH OPTIONS = {'case_sensitive': 'false', 'normalize': 'true'};

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

-- SAI index on posts_by_id.content for content search (LIKE prefix/contains matching)
DROP INDEX IF EXISTS post_content_sai_idx;
CREATE CUSTOM INDEX post_content_sai_idx 
  ON posts_by_id (content) 
  USING 'StorageAttachedIndex' 
  WITH OPTIONS = {'case_sensitive': 'false', 'normalize': 'true'};

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
    updated_at TIMESTAMP,
    is_deleted BOOLEAN,
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
    updated_at TIMESTAMP,
    is_deleted BOOLEAN,
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

-- ============== PASSWORD RESET ==============
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    reset_token TEXT PRIMARY KEY,
    user_id UUID,
    expires_at TIMESTAMP,
    used BOOLEAN,
    created_at TIMESTAMP
) WITH default_time_to_live = 3600;

-- ============== CONTENT MODERATION ==============
CREATE TABLE IF NOT EXISTS reports (
    id UUID,
    reporter_id UUID,
    target_type TEXT,          -- 'post', 'comment', 'user'
    target_id UUID,
    reason TEXT,               -- 'spam', 'harassment', 'inappropriate', 'other'
    description TEXT,
    status TEXT,               -- 'pending', 'reviewed', 'resolved', 'dismissed'
    created_at TIMESTAMP,
    PRIMARY KEY ((target_type, target_id), id)
);

CREATE TABLE IF NOT EXISTS reports_by_user (
    reporter_id UUID,
    target_type TEXT,
    target_id UUID,
    report_id UUID,
    created_at TIMESTAMP,
    PRIMARY KEY ((reporter_id), created_at, report_id)
) WITH CLUSTERING ORDER BY (created_at DESC, report_id ASC);

CREATE TABLE IF NOT EXISTS blocks (
    blocker_id UUID,
    blocked_id UUID,
    created_at TIMESTAMP,
    PRIMARY KEY ((blocker_id), blocked_id)
);

CREATE TABLE IF NOT EXISTS blocked_by (
    blocked_id UUID,
    blocker_id UUID,
    created_at TIMESTAMP,
    PRIMARY KEY ((blocked_id), blocker_id)
);

CREATE TABLE IF NOT EXISTS mutes (
    muter_id UUID,
    muted_id UUID,
    created_at TIMESTAMP,
    PRIMARY KEY ((muter_id), muted_id)
);

-- ============== LOCATION NAMES CACHE ==============
CREATE TABLE IF NOT EXISTS location_names (
    geohash_prefix TEXT PRIMARY KEY,
    display_name TEXT,
    name TEXT,
    village TEXT,
    city_district TEXT,
    city TEXT,
    state TEXT,
    region TEXT,
    postcode TEXT,
    country TEXT,
    country_code TEXT,
    latitude DOUBLE,
    longitude DOUBLE,
    created_at TIMESTAMP
);

-- ============== NOTIFICATIONS V2 ==============
CREATE TABLE IF NOT EXISTS notifications_by_user (
    user_id        UUID,
    created_at     TIMESTAMP,
    notification_id UUID,
    type           TEXT,
    actor_id       UUID,
    target_id      UUID,
    target_type    TEXT,
    message        TEXT,
    payload        MAP<TEXT, TEXT>,
    is_read        BOOLEAN,
    is_deleted     BOOLEAN,
    PRIMARY KEY ((user_id), created_at, notification_id)
) WITH CLUSTERING ORDER BY (created_at DESC, notification_id ASC)
  AND default_time_to_live = 7776000
  AND gc_grace_seconds = 864000;

-- ============== PUSH DEVICE TOKENS ==============
CREATE TABLE IF NOT EXISTS push_device_tokens (
    user_id      UUID,
    device_id    TEXT,
    platform     TEXT,
    fcm_token    TEXT,
    app_version  TEXT,
    created_at   TIMESTAMP,
    last_seen_at TIMESTAMP,
    PRIMARY KEY ((user_id), device_id)
);

CREATE TABLE IF NOT EXISTS device_tokens_by_token (
    fcm_token    TEXT,
    user_id      UUID,
    device_id    TEXT,
    PRIMARY KEY ((fcm_token))
);

-- ============== NOTIFICATION PREFERENCES ==============
CREATE TABLE IF NOT EXISTS notification_preferences (
    user_id              UUID PRIMARY KEY,
    like_in_app          BOOLEAN,
    like_push            BOOLEAN,
    comment_in_app       BOOLEAN,
    comment_push         BOOLEAN,
    follow_in_app        BOOLEAN,
    follow_push          BOOLEAN,
    nearby_post_in_app   BOOLEAN,
    nearby_post_push     BOOLEAN,
    mention_in_app       BOOLEAN,
    mention_push         BOOLEAN,
    quiet_start          TEXT,
    quiet_end            TEXT,
    timezone             TEXT,
    updated_at           TIMESTAMP
);
