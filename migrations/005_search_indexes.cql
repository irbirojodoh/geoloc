-- Migration: 005_search_indexes.cql
-- Run after 004_like_comment_v3.cql

USE geoloc;

-- ============== SEARCH INDEXES ==============

-- SAI index on posts_by_id.content for content search
-- Using 'normalize' and 'case_sensitive: false' for case-insensitive matching
CREATE CUSTOM INDEX IF NOT EXISTS post_content_sai_idx 
  ON posts_by_id (content) 
  USING 'StorageAttachedIndex' 
  WITH OPTIONS = {'case_sensitive': 'false', 'normalize': 'true'};
