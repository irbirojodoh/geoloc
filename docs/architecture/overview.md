# System Architecture

High-level overview of the Geoloc system architecture.

## Components

```
┌───────────────────────────────────────────────────────────────────────┐
│                          Mobile Clients                                │
│                    (iOS / Android / Flutter)                           │
└───────────────────────────────┬───────────────────────────────────────┘
                                │ HTTPS
                                ▼
┌───────────────────────────────────────────────────────────────────────┐
│                           Load Balancer                                │
│                        (nginx / Cloud LB)                              │
└───────────────────────────────┬───────────────────────────────────────┘
                                │
                                ▼
┌───────────────────────────────────────────────────────────────────────┐
│                          Go API Server                                 │
│                         (Gin Framework)                                │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────┐ │
│  │   Auth   │  │  Posts   │  │  Users   │  │  Search  │  │  Geo   │ │
│  │ Handlers │  │ Handlers │  │ Handlers │  │ Handlers │  │Handlers│ │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘  └────────┘ │
│  ┌──────────┐  ┌──────────────────────────────────────────────────┐ │
│  │    DM    │  │                    Middleware Layer               │ │
│  │ Handlers │  │   (Auth, Rate Limiting, CORS, Request Logging)    │ │
│  └──────────┘  └──────────────────────────────────────────────────┘ │
└───────────────┬───────────────────┬───────────────────┬───────────────┘
                │                   │                   │
          ┌─────┴─────┐       ┌─────┴─────┐       ┌─────┴─────┐
          ▼           ▼       ▼           ▼       ▼           ▼
┌─────────────────┐  ┌──────────────┐  ┌─────────────────────────┐
│    Cassandra    │  │ Elasticsearch│  │     Nominatim API       │
│    Database     │  │  (Full-text) │  │   (Reverse Geocoding)   │
└─────────────────┘  └──────────────┘  └─────────────────────────┘
                          ▲
                          │ Index (posts.created topic)
                    ┌─────┴─────┐
                    │  Search   │
                    │  Indexer  │
                    │ (Kafka    │
                    │ Consumer) │
                    └───────────┘
```

## Technology Stack

| Layer | Technology |
|-------|------------|
| API Language | Go 1.24+ |
| Web Framework | Gin |
| Database | Apache Cassandra 4.1 |
| Search Engine | Elasticsearch 8.13+ |
| Message Broker | Apache Kafka (KRaft mode) |
| Cache & Pub/Sub | Redis 7+ |
| Full-Text Search | Elasticsearch with edge n-gram analyzers |
| Autocomplete | Redis sorted sets (ZRANGEBYLEX) |
| Authentication | JWT (HS256) |
| File Storage | Local filesystem |
| Geocoding | Nominatim (OSM) |
| Containerization | Docker |

## Data Flow

### Feed Request

1. Client sends location coordinates + JWT token
2. Server validates JWT
3. Server calculates geohash prefix from coordinates
4. Queries Cassandra `posts_by_geohash` table for nearby cells
5. Filters posts by distance
6. Enriches with user info and location names
7. Returns paginated response

### Post Creation

1. Client uploads media via `/upload/post` (optional)
2. Client creates post with content + coordinates
3. Server generates geohash and inserts into denormalized Cassandra tables
4. API publishes `posts.created` to Kafka (when `KAFKA_BROKERS` is configured)
5. `search-indexer` consumes the event and indexes the post in Elasticsearch
6. Returns created post to client

For posts created before indexing was enabled, run `go run cmd/backfill-search/main.go` once.

### Direct message (E2EE)

1. Client generates X25519 keys locally and registers public key via `PUT /api/v1/dm/keys`
2. Client opens shared SSE stream (`GET /api/v1/notifications/stream`)
3. Client creates or resumes 1:1 conversation; encrypts messages client-side (ECDH + HKDF + AES-GCM)
4. Server stores ciphertext only, fans out via Redis `dm:{userID}` to SSE; optionally publishes to Kafka `dm_messages` when recipient is offline
5. History and multi-device: versioned public keys + optional identity backup (`migrations/007_dm.cql`, `008_dm_multidevice.cql`)

See [Direct messages architecture](./dm.md) for full design.

## Scalability

### Cassandra
- Designed for horizontal scaling
- Denormalized tables optimize for read patterns
- Geohash partitioning distributes data by location

### API Server
- Stateless design enables horizontal scaling
- JWT tokens require no server-side session storage

### Rate Limiting
- Per-IP rate limiting (100 req/min; 1000/min in development)
- DM write routes: 60 req/min per authenticated user
- Nominatim API: 1 req/sec (cached)

## Further reading

- [Direct messages (E2EE)](./dm.md) — ciphertext relay, SSE delivery, multi-device
- [Database schema](./database.md) — Cassandra tables including DM
- [Geohashing](./geohashing.md) — location-based feed queries
