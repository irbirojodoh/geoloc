-- MVP Features Migration
-- Apply with: cqlsh -f migrations/003_mvp_features.cql

USE geoloc;

-- ============== PASSWORD RESET ==============
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    reset_token TEXT PRIMARY KEY,
    user_id UUID,
    expires_at TIMESTAMP,
    used BOOLEAN,
    created_at TIMESTAMP
) WITH default_time_to_live = 3600; -- Auto-expire rows after 1 hour

-- ============== ACCOUNT SOFT DELETE ==============
-- Add soft-delete columns to users table
ALTER TABLE users ADD is_deleted BOOLEAN;
ALTER TABLE users ADD deleted_at TIMESTAMP;

-- ============== CONTENT MODERATION ==============

-- Reports (users reporting content or other users)
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

-- Reports by reporter (for duplicate-check and history)
CREATE TABLE IF NOT EXISTS reports_by_user (
    reporter_id UUID,
    target_type TEXT,
    target_id UUID,
    report_id UUID,
    created_at TIMESTAMP,
    PRIMARY KEY ((reporter_id), created_at, report_id)
) WITH CLUSTERING ORDER BY (created_at DESC, report_id ASC);

-- Blocks (user A blocks user B)
CREATE TABLE IF NOT EXISTS blocks (
    blocker_id UUID,
    blocked_id UUID,
    created_at TIMESTAMP,
    PRIMARY KEY ((blocker_id), blocked_id)
);

-- Reverse lookup for block checks
CREATE TABLE IF NOT EXISTS blocked_by (
    blocked_id UUID,
    blocker_id UUID,
    created_at TIMESTAMP,
    PRIMARY KEY ((blocked_id), blocker_id)
);

-- Mutes (user A mutes user B — hide from feed, no hard block)
CREATE TABLE IF NOT EXISTS mutes (
    muter_id UUID,
    muted_id UUID,
    created_at TIMESTAMP,
    PRIMARY KEY ((muter_id), muted_id)
);
