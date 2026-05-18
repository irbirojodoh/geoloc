# Search API

Geoloc has two search tiers:

1. **Legacy Cassandra-backed search** — basic prefix matching via SAI indexes (`/api/v1/search/users`, `/api/v1/search/posts`)
2. **Elasticsearch-backed search** — full-text search with fuzzy matching, geo-filtering, and autocomplete (`/api/v1/search`, `/api/v1/search/nearby`, `/api/v1/autocomplete`)

---

## Legacy Cassandra Search

### Search Users

**Endpoint:** `GET /api/v1/search/users`

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `q` | string | Yes | Search query (min 2 chars) |
| `limit` | int | No | Max results (default: 20, max: 50) |

**Response:** `200 OK`
```json
{
  "query": "john",
  "count": 2,
  "results": [
    {
      "id": "user-uuid",
      "username": "john_doe",
      "full_name": "John Doe",
      "profile_picture_url": "..."
    }
  ]
}
```

### Search Posts

**Endpoint:** `GET /api/v1/search/posts`

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `q` | string | Yes | Search query (min 2 chars) |
| `limit` | int | No | Max results (default: 20, max: 50) |

**Response:** `200 OK`
```json
{
  "query": "sunset",
  "count": 5,
  "results": [
    {
      "id": "post-uuid",
      "content": "Matching content...",
      "geohash": "qqggy",
      "created_at": "2026-01-05T10:30:00Z"
    }
  ]
}
```

---

## Elasticsearch Search (New)

### Global Search

**Endpoint:** `GET /api/v1/search`

Performs full-text search across posts and users simultaneously using Elasticsearch.

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `q` | string | Yes | Search query (min 1 char) |
| `type` | string | No | Scope: `all`, `posts`, or `users` (default: `all`) |
| `page` | int | No | Page number (default: 1) |
| `limit` | int | No | Results per page (default: 20, max: 50) |

**Response:** `200 OK`

Each item in `posts` uses the same `data.Post` shape as `GET /api/v1/feed` (`username`, `profile_picture_url`, `location_name`, `address`, `like_count`, `is_liked`, etc.). Posts are hydrated from Cassandra after Elasticsearch returns matching IDs.

```json
{
  "posts": [
    {
      "id": "post-uuid",
      "user_id": "user-uuid",
      "username": "john_doe",
      "profile_picture_url": "https://...",
      "content": "Matching content...",
      "media_urls": ["https://..."],
      "geohash": "qqggy",
      "location_name": "Depok",
      "address": { "city": "Depok", "country": "Indonesia" },
      "like_count": 42,
      "is_liked": false,
      "created_at": "2026-01-05T10:30:00Z"
    }
  ],
  "users": [
    {
      "id": "user-uuid",
      "username": "john_doe",
      "full_name": "John Doe",
      "profile_picture_url": "..."
    }
  ],
  "total": 7,
  "query": "coffee"
}
```

### Nearby Search

**Endpoint:** `GET /api/v1/search/nearby`

Full-text search filtered by geographic proximity. Users are not geo-filtered.

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `q` | string | Yes | Search query |
| `lat` | float | Yes | User latitude |
| `lon` | float | Yes | User longitude |
| `radius_km` | float | No | Search radius in km (default: 5) |
| `type` | string | No | Scope: `all`, `posts`, or `users` (default: `all`) |

**Ranking:** Results are re-ranked using a weighted formula:
- 50% Elasticsearch relevance score
- 30% Recency (1 / (1 + hours_since_posted))
- 20% Proximity (1 / (1 + distance_km))

**Response:** `200 OK`

Post objects match the feed shape; `distance_km` is set from the search `lat`/`lon` query parameters.

```json
{
  "posts": [ ... ],
  "users": [ ... ],
  "total": 7,
  "query": "coffee"
}
```

### Autocomplete

**Endpoint:** `GET /api/v1/autocomplete`

Provides real-time suggestions for usernames (Redis sorted sets) and hashtags (Elasticsearch aggregations).

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `q` | string | Yes | Prefix to autocomplete (min 1 char) |
| `type` | string | No | Scope: `all`, `users`, or `hashtags` (default: `all`) |

**Response:** `200 OK`
```json
{
  "users": ["johndoe", "johnsmith"],
  "hashtags": ["#coffee", "#coffeeshop"]
}
```

---

## Architecture

```
┌───────────────────────────────────────────────────────────────┐
│                        Client Request                          │
│  GET /api/v1/search?q=coffee&type=all                         │
└───────────┬───────────────────────────────────────────────────┘
            │
            ▼
┌───────────────────────────────────────────────────────────────┐
│                    Search Handler (Fan-out)                    │
│                                                               │
│   ┌──────────────┐          ┌──────────────┐                  │
│   │  SearchPosts  │          │  SearchUsers  │                 │
│   │  (goroutine)  │          │  (goroutine)  │                 │
│   └──────┬───────┘          └──────┬───────┘                  │
│          │                         │                          │
└──────────┼─────────────────────────┼──────────────────────────┘
           │                         │
           ▼                         ▼
┌──────────────────┐    ┌──────────────────┐
│  Elasticsearch   │    │  Elasticsearch   │
│  posts index     │    │  users index     │
│  (multi_match +  │    │  (function_score │
│   geo_distance)  │    │   + fuzziness)   │
└────────┬─────────┘    └────────┬─────────┘
         │                       │
         ▼                       ▼
┌──────────────────┐    ┌──────────────────┐
│  Hydrate from    │    │  Hydrate from    │
│  Cassandra       │    │  Cassandra       │
│  (posts_by_id)   │    │  (users table)   │
│  (10 concurrent) │    │  (10 concurrent) │
└────────┬─────────┘    └────────┬─────────┘
         │                       │
         └───────────┬───────────┘
                     ▼
          ┌──────────────────┐
          │  Apply Ranker    │
          │  (if geo context)│
          └──────────────────┘
```

---

## Indexing Pipeline

```
Post Created ──► API publishes posts.created ──► Kafka ──► Search Indexer ──► Elasticsearch
                                                              │
                                                              └──► Redis ZADD (username autocomplete)
```

### How posts reach Elasticsearch

| Source | Mechanism |
|--------|-----------|
| **New posts** | API publishes a `PostCreatedEvent` to Kafka topic `posts.created` when `KAFKA_BROKERS` is set. The `search-indexer` consumer indexes each message into Elasticsearch. |
| **Existing posts** | The indexer does **not** scan Cassandra. Run the one-off backfill command instead (see below). |

The API producer is enabled when `KAFKA_BROKERS` is configured (see [Environment Configuration](../environment.md)). Notification Kafka (`KAFKA_NOTIFICATIONS_ENABLED`) is separate but uses the same broker.

The `search-indexer` is a standalone Go service (`cmd/indexer/main.go`) that:
- Creates ES indexes on startup if missing (`posts`, `users`)
- Consumes the `posts.created` Kafka topic (consumer group: `search-indexer`)
- Indexes each post into Elasticsearch (idempotent — uses `post_id` as `_id`)
- Syncs the post author's username into Redis sorted set `users:autocomplete`
- Retries on transient Kafka/ES errors (does not exit on failure)

### Local development setup

1. Start infrastructure:
   ```bash
   docker compose --profile dev up -d
   ```

2. Configure `.env.development` (loaded automatically when `APP_ENV=development`):
   ```env
   KAFKA_BROKERS=127.0.0.1:9092
   KAFKA_NOTIFICATIONS_ENABLED=true
   ELASTICSEARCH_URL=http://localhost:9200
   ELASTICSEARCH_INDEX_POSTS=posts
   ELASTICSEARCH_INDEX_USERS=users
   ```

3. Run the indexer (separate terminal):
   ```bash
   go run cmd/indexer/main.go
   ```

4. Run the API:
   ```bash
   go run cmd/api/main.go
   ```

5. Backfill posts that existed before indexing was enabled:
   ```bash
   go run cmd/backfill-search/main.go
   ```

### Backfill existing posts

```bash
go run cmd/backfill-search/main.go
```

This reads all rows from Cassandra `posts_by_id`, writes them directly to Elasticsearch, and syncs usernames to Redis autocomplete. Safe to re-run — documents are upserted by `post_id`.

### Verify indexing

```bash
# Document count
curl -s http://localhost:9200/posts/_count

# Sample search
curl -s 'http://localhost:9200/posts/_search?q=jakarta&pretty'

# Kafka topic offsets
docker exec geoloc-kafka kafka-get-offsets --bootstrap-server localhost:29092 --topic posts.created
```

### Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `/api/v1/search` returns empty `posts` | ES index empty | Run `go run cmd/backfill-search/main.go` |
| New posts not searchable | Indexer not running or API not publishing | Restart API + indexer; confirm `KAFKA_BROKERS` is set |
| `Group Coordinator Not Available` at indexer startup | Kafka still initializing | Wait for `geoloc-kafka` healthy, restart indexer |
| `i/o timeout` on empty `posts.created` topic | Idle consumer long-poll | Harmless while waiting; create a test post to confirm flow |

---

## Best Practices

- **Debounce**: Wait 300-500ms after user stops typing before searching
- **Min Length**: Start search at 1+ characters (autocomplete), 2+ for legacy
- **Global vs Nearby**: Use global search for text-only queries; use nearby for location-aware results
- **Caching**: Client-side cache identical queries for 30-60 seconds
- **Fallback**: The legacy Cassandra `/api/v1/search/users` and `/api/v1/search/posts` remain available if ES is down
