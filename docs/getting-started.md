# Quick Start Guide

Get Geoloc running locally in under 10 minutes.

## Prerequisites

- Go 1.21+
- Docker & Docker Compose
- Git

## 1. Clone & Setup

```bash
git clone <repository-url>
cd geoloc
```

Environment files are loaded from `.env.development` by default (see [Environment Configuration](./environment.md)).

## 2. Start Infrastructure

```bash
docker compose --profile dev up -d
```

This starts Cassandra (3-node cluster), Redis, Kafka, Elasticsearch, and Kafka UI.

Wait until core services are healthy:

```bash
docker ps
```

Schema is applied automatically by the `cassandra-db-init` service on first boot.

## 3. Configure Search (recommended)

Add to `.env.development`:

```env
KAFKA_BROKERS=127.0.0.1:9092
KAFKA_NOTIFICATIONS_ENABLED=true
ELASTICSEARCH_URL=http://localhost:9200
ELASTICSEARCH_INDEX_POSTS=posts
ELASTICSEARCH_INDEX_USERS=users
```

## 4. Run the Search Indexer

In a **separate terminal**:

```bash
go run cmd/indexer/main.go
```

Leave this running so new posts are indexed into Elasticsearch.

## 5. Run the API

```bash
go run cmd/api/main.go
```

The API will be available at `http://localhost:8080`.

You should see `Kafka Search Indexer Producer enabled` when `KAFKA_BROKERS` is set.

## 6. Backfill Existing Posts (optional)

If you have posts in Cassandra from before search indexing was enabled:

```bash
go run cmd/backfill-search/main.go
```

## 7. Verify

```bash
# Health check
curl http://localhost:8080/health

# Register a user
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "email": "test@example.com",
    "password": "password123",
    "full_name": "Test User"
  }'

# Elasticsearch post count (after backfill or new posts)
curl -s http://localhost:9200/posts/_count
```

---

## Next Steps

- [Environment Configuration](./environment.md) - Configure environment variables
- [Search API](./api/search.md) - Elasticsearch search and indexing pipeline
- [API Overview](./api/README.md) - Learn about the API
- [Direct messages (E2EE)](./api/dm.md) - Encrypted DMs (`migrations/007_dm.cql`, `008_dm_multidevice.cql`)
- [Docker Deployment](./deployment/docker.md) - Full Docker Compose reference
- [Flutter Client Guide](./client/flutter.md) - Build the mobile app
