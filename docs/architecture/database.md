# Database Schema

Geoloc uses Apache Cassandra with a denormalized schema optimized for specific query patterns.

## Design Philosophy

Cassandra requires designing tables around **query patterns**, not normalization. Each query pattern gets its own table.

## Tables

### users

Stores user profiles.

```cql
CREATE TABLE users (
    id UUID PRIMARY KEY,
    username TEXT,
    email TEXT,
    password_hash TEXT,
    full_name TEXT,
    bio TEXT,
    phone_number TEXT,
    profile_picture_url TEXT,
    last_online TIMESTAMP,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);
```

**Indexes:**
- `users_by_username` - Lookup by username
- `users_by_email` - Lookup by email

### posts_by_id

Lookup posts by ID.

```cql
CREATE TABLE posts_by_id (
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
```

### posts_by_geohash

**Primary query table** for location-based feed.

```cql
CREATE TABLE posts_by_geohash (
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
    PRIMARY KEY (geohash_prefix, created_at, post_id)
) WITH CLUSTERING ORDER BY (created_at DESC, post_id ASC);
```

**Query pattern:** Get posts in a location area, ordered by newest first.

### posts_by_user

User profile posts timeline.

```cql
CREATE TABLE posts_by_user (
    user_id UUID,
    created_at TIMESTAMP,
    post_id UUID,
    content TEXT,
    media_urls LIST<TEXT>,
    latitude DOUBLE,
    longitude DOUBLE,
    ip_address TEXT,
    user_agent TEXT,
    PRIMARY KEY (user_id, created_at, post_id)
) WITH CLUSTERING ORDER BY (created_at DESC, post_id ASC);
```

**Query pattern:** Get all posts by a user, ordered by newest first.

### location_names

Cache for reverse geocoding results.

```cql
CREATE TABLE location_names (
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
```

**Purpose:** Avoid redundant Nominatim API calls. Cache location data by geohash.

## Direct messages

End-to-end encrypted 1:1 messaging. The server stores **ciphertext and public keys only** — never plaintext or private keys.

**Migrations:** `007_dm.cql`, `008_dm_multidevice.cql`

### user_public_keys

Versioned X25519 public keys for ECDH on the client.

```cql
CREATE TABLE user_public_keys (
    user_id     UUID,
    key_version INT,
    public_key  TEXT,
    created_at  TIMESTAMP,
    PRIMARY KEY (user_id, key_version)
) WITH CLUSTERING ORDER BY (key_version DESC);
```

**Query patterns:** Current key (first row); specific version; list all versions for history decryption.

### user_dm_identity_backups

Passphrase-wrapped identity backup (opaque blob — client encrypts before upload).

```cql
CREATE TABLE user_dm_identity_backups (
    user_id         UUID,
    backup_version  INT,
    ciphertext      TEXT,
    nonce           TEXT,
    kdf_salt        TEXT,
    updated_at      TIMESTAMP,
    PRIMARY KEY (user_id, backup_version)
) WITH CLUSTERING ORDER BY (backup_version DESC);
```

**Query pattern:** Latest backup for multi-device restore (`LIMIT 1` or highest `backup_version`).

### dm_conversations

Canonical row per 1:1 thread. `conversation_id` is deterministic (UUIDv5-style from sorted participant pair).

```cql
CREATE TABLE dm_conversations (
    conversation_id UUID,
    participant_a   UUID,
    participant_b   UUID,
    created_at      TIMESTAMP,
    last_message_at TIMESTAMP,
    PRIMARY KEY (conversation_id)
);
```

### dm_conversations_by_user

Per-user inbox for listing chats without scanning messages.

```cql
CREATE TABLE dm_conversations_by_user (
    user_id         UUID,
    last_message_at TIMESTAMP,
    conversation_id UUID,
    other_user_id   UUID,
    PRIMARY KEY (user_id, last_message_at, conversation_id)
) WITH CLUSTERING ORDER BY (last_message_at DESC, conversation_id ASC);
```

**Query pattern:** Paginated inbox ordered by recent activity. Rows can be deleted per-user (inbox hide) without affecting the peer.

### dm_messages

Encrypted message bodies. Clustered by `message_id` TIMEUUID descending (newest first).

```cql
CREATE TABLE dm_messages (
    conversation_id     UUID,
    message_id          TIMEUUID,
    sender_id           UUID,
    ciphertext          TEXT,
    nonce               TEXT,
    key_version         INT,
    sender_key_version  INT,
    sent_at             TIMESTAMP,
    deleted_at          TIMESTAMP,
    PRIMARY KEY (conversation_id, message_id)
) WITH CLUSTERING ORDER BY (message_id DESC);
```

| Column | Meaning |
|--------|---------|
| `key_version` | Recipient’s public key version used when encrypting |
| `sender_key_version` | Sender’s public key version at send time (for history decrypt) |
| `deleted_at` | Soft-delete tombstone (sender-only) |

### dm_read_receipts

Per-participant read cursor in a conversation.

```cql
CREATE TABLE dm_read_receipts (
    conversation_id UUID,
    user_id         UUID,
    last_read_id    TIMEUUID,
    read_at         TIMESTAMP,
    PRIMARY KEY (conversation_id, user_id)
);
```

**Design note:** Send path uses a Cassandra **logged batch** to insert the message, bump `last_message_at`, and refresh both users’ inbox rows atomically.

See [Direct messages architecture](./dm.md) for data flows and component layout.

## Social Tables

### follows

```cql
CREATE TABLE follows (
    follower_id UUID,
    following_id UUID,
    created_at TIMESTAMP,
    PRIMARY KEY (follower_id, following_id)
);
```

### comments_by_post

Nested comments with depth tracking.

```cql
CREATE TABLE comments_by_post (
    post_id UUID,
    comment_id UUID,
    parent_id UUID,
    user_id UUID,
    content TEXT,
    depth INT,
    created_at TIMESTAMP,
    PRIMARY KEY (post_id, created_at, comment_id)
) WITH CLUSTERING ORDER BY (created_at DESC, comment_id ASC);
```

### notifications

```cql
CREATE TABLE notifications (
    user_id UUID,
    notification_id UUID,
    type TEXT,
    actor_id UUID,
    target_type TEXT,
    target_id UUID,
    message TEXT,
    is_read BOOLEAN,
    created_at TIMESTAMP,
    PRIMARY KEY (user_id, created_at, notification_id)
) WITH CLUSTERING ORDER BY (created_at DESC, notification_id ASC);
```

## Key Design Decisions

1. **Denormalization**: Same data in multiple tables for different query patterns
2. **Geohash Partitioning**: Location queries are efficient by querying specific geohash cells
3. **Time-sorted clustering**: Most tables use `created_at DESC` for timeline ordering
4. **UUIDs everywhere**: TimeUUIDs for ordering, regular UUIDs for references
