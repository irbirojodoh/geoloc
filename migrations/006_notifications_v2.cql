-- Notification System V2 Migration
-- Apply with: cqlsh -f migrations/006_notifications_v2.cql

USE geoloc;

-- ============== NOTIFICATIONS V2 ==============
-- Enhanced notifications table with payload map, soft-delete, and 90-day TTL
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

-- Reverse lookup: token → user for FCM error handling
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
