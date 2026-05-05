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
│  ┌──────────────────────────────────────────────────────────────────┐ │
│  │                    Middleware Layer                               │ │
│  │   (Auth, Rate Limiting, CORS, Request Logging, Timeout)          │ │
│  └──────────────────────────────────────────────────────────────────┘ │
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

1. Client uploads media via `/upload/post`
2. Client creates post with media URLs + coordinates
3. Server generates geohash
4. Inserts into 3 Cassandra tables (denormalized)
5. Returns created post

## Scalability

### Cassandra
- Designed for horizontal scaling
- Denormalized tables optimize for read patterns
- Geohash partitioning distributes data by location

### API Server
- Stateless design enables horizontal scaling
- JWT tokens require no server-side session storage

### Rate Limiting
- Per-IP rate limiting (100 req/min)
- Nominatim API: 1 req/sec (cached)
