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
