-- Keyspace
CREATE KEYSPACE IF NOT EXISTS geoloc 
WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};

USE geoloc;

-- Users table (with auth)
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    username TEXT,
    email TEXT,
    full_name TEXT,
    bio TEXT,
    phone_number TEXT,
    profile_picture_url TEXT,
    password_hash TEXT,
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
    PRIMARY KEY ((user_id), created_at, post_id)
) WITH CLUSTERING ORDER BY (created_at DESC, post_id ASC);
