-- Migration: like_schema_v2.cql
-- High-concurrency like system with LWT support
-- Run after cassandra_schema.cql

USE geoloc;

-- Like State table for LWT-based idempotent operations
-- This is the source of truth for who liked what
-- Uses Cassandra LWT (Lightweight Transactions) for atomic state changes
CREATE TABLE IF NOT EXISTS like_state (
    target_type TEXT,          -- 'post' or 'comment'
    target_id UUID,            -- post_id or comment_id
    user_id UUID,
    created_at TIMESTAMP,
    PRIMARY KEY ((target_type, target_id), user_id)
);

-- Note: like_counts table is now deprecated
-- Counters are stored in Redis for faster atomic operations
-- The existing like_counts table can be kept for backup/migration purposes

-- Index for finding all likes by a user (optional, for "posts you've liked" feature)
CREATE TABLE IF NOT EXISTS likes_by_user (
    user_id UUID,
    target_type TEXT,
    target_id UUID,
    created_at TIMESTAMP,
    PRIMARY KEY ((user_id), created_at, target_id)
) WITH CLUSTERING ORDER BY (created_at DESC, target_id ASC);
