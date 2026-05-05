# Search API

Geoloc has two search tiers:

1. **Legacy Cassandra-backed search** — basic prefix matching via SAI indexes (`/api/v1/search/users`, `/api/v1/search/posts`)
2. **Elasticsearch-backed search** — full-text search with fuzzy matching, geo-filtering, and autocomplete (`/api/v1/search`, `/api/v1/search/nearby`, `/v1/autocomplete`)

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
```json
{
  "posts": [
    {
      "id": "post-uuid",
      "user_id": "user-uuid",
      "content": "Matching content...",
      "geohash": "qqggy",
      "created_at": "2026-01-05T10:30:00Z",
      "like_count": 42
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
Post Created ──► Kafka (posts.created) ──► Search Indexer ──► Elasticsearch
                                              │
                                              └──► Redis ZADD (username autocomplete)
```

The `search-indexer` is a standalone Go service that:
- Consumes the `posts.created` Kafka topic (consumer group: `search-indexer`)
- Indexes each post into Elasticsearch (idempotent — uses `post_id` as `_id`)
- Syncs the post author's username into Redis sorted set `users:autocomplete`
- Handles transient errors with exponential backoff (does not exit on failure)

---

## Best Practices

- **Debounce**: Wait 300-500ms after user stops typing before searching
- **Min Length**: Start search at 1+ characters (autocomplete), 2+ for legacy
- **Global vs Nearby**: Use global search for text-only queries; use nearby for location-aware results
- **Caching**: Client-side cache identical queries for 30-60 seconds
- **Fallback**: The legacy Cassandra `/api/v1/search/users` and `/api/v1/search/posts` remain available if ES is down
