-- Migration: 004_like_comment_v3.cql
-- Run after 003_mvp_features.cql

USE geoloc;

-- ============== COMMENTS V2 ==============

-- Add updated_at and is_deleted for edit + soft-delete
ALTER TABLE comments ADD updated_at TIMESTAMP;
ALTER TABLE comments ADD is_deleted BOOLEAN;

ALTER TABLE comments_by_id ADD updated_at TIMESTAMP;
ALTER TABLE comments_by_id ADD is_deleted BOOLEAN;

-- ============== COMMENT COUNTS → REDIS ==============
-- Migrate comment counts from Cassandra counters to Redis
-- The existing comment_counts table is kept for backup/migration purposes
