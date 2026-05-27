-- Direct messaging (E2EE ciphertext only)
-- Apply with: cqlsh -f migrations/007_dm.cql

USE geoloc;

-- Store each user's public keys (versioned)
CREATE TABLE IF NOT EXISTS user_public_keys (
    user_id     UUID,
    key_version INT,
    public_key  TEXT,
    created_at  TIMESTAMP,
    PRIMARY KEY (user_id, key_version)
) WITH CLUSTERING ORDER BY (key_version DESC);

-- One row per DM conversation (canonical: smaller UUID first in participant_a/b)
CREATE TABLE IF NOT EXISTS dm_conversations (
    conversation_id UUID,
    participant_a   UUID,
    participant_b   UUID,
    created_at      TIMESTAMP,
    last_message_at TIMESTAMP,
    PRIMARY KEY (conversation_id)
);

-- Inbox / conversation list per user
CREATE TABLE IF NOT EXISTS dm_conversations_by_user (
    user_id         UUID,
    last_message_at TIMESTAMP,
    conversation_id UUID,
    other_user_id   UUID,
    PRIMARY KEY (user_id, last_message_at, conversation_id)
) WITH CLUSTERING ORDER BY (last_message_at DESC, conversation_id ASC);

-- Messages (ciphertext only)
CREATE TABLE IF NOT EXISTS dm_messages (
    conversation_id UUID,
    message_id      TIMEUUID,
    sender_id       UUID,
    ciphertext      TEXT,
    nonce           TEXT,
    key_version     INT,
    sent_at         TIMESTAMP,
    deleted_at      TIMESTAMP,
    PRIMARY KEY (conversation_id, message_id)
) WITH CLUSTERING ORDER BY (message_id DESC);

-- Read receipts
CREATE TABLE IF NOT EXISTS dm_read_receipts (
    conversation_id UUID,
    user_id         UUID,
    last_read_id    TIMEUUID,
    read_at         TIMESTAMP,
    PRIMARY KEY (conversation_id, user_id)
);
