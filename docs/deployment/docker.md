# Docker Deployment

Run Geoloc locally using Docker Compose.

## Prerequisites

- Docker 20.10+
- Docker Compose 2.0+

## Quick Start

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

## Services

The `docker-compose.yml` includes:

| Service | Port | Description |
|---------|------|-------------|
| `cassandra-1/2/3` | 9042 | Cassandra cluster (3 nodes) |
| `redis` | 6379 | Redis cache & pub/sub |
| `kafka` | 9092 | Apache Kafka broker (KRaft mode) |
| `kafka-ui` | 8090 | Kafka management UI |
| `elasticsearch` | 9200 | Elasticsearch full-text search engine |
| `search-indexer` | — | Kafka consumer: indexes posts into ES + syncs Redis autocomplete |
| `api` | 8080 | Go API server |
| `caddy` | 80/443 | Reverse proxy with auto TLS |

## Initialize Database

After starting Cassandra, apply the schema. With Docker Compose, the `cassandra-db-init` service handles this automatically. To run manually:

```bash
# Wait for Cassandra to be ready (~30 seconds)
docker compose exec cassandra-1 cqlsh -e "DESCRIBE KEYSPACES"

# Apply schema
docker cp migrations/cassandra_schema.cql geoloc_cassandra-1:/tmp/
docker exec geoloc_cassandra-1 cqlsh -f /tmp/cassandra_schema.cql

# Apply additional migrations
docker exec geoloc_cassandra-1 cqlsh -f /tmp/003_mvp_features.cql

# Optional: Seed test data
docker cp migrations/seed_test_data.cql geoloc_cassandra-1:/tmp/
docker exec geoloc_cassandra-1 cqlsh -f /tmp/seed_test_data.cql
```

## Development Mode

Run API locally with Docker services:

```bash
# Start infrastructure (Cassandra, Redis, Kafka, Elasticsearch)
docker compose up -d cassandra-1 redis kafka elasticsearch

# Run API with live reload
go run cmd/api/main.go

# Run search indexer (in a separate terminal)
go run cmd/indexer/main.go
```

## Data Persistence

Cassandra data is persisted in a Docker volume:

```yaml
volumes:
  cassandra_data:
```

To reset the database:

```bash
docker-compose down -v  # -v removes volumes
docker-compose up -d
# Re-apply schema
```

## Environment Variables

Pass to API container via `docker-compose.yml`:

```yaml
api:
  environment:
    - CASSANDRA_HOST=cassandra-1
    - CASSANDRA_KEYSPACE=geoloc
    - REDIS_HOST=redis
    - KAFKA_BROKERS=kafka:29092
    - JWT_SECRET=your-secret-key
    - ELASTICSEARCH_URL=http://elasticsearch:9200
```

The search-indexer also reads:

| Variable | Description |
|----------|-------------|
| `ELASTICSEARCH_URL` | Elasticsearch HTTP endpoint |
| `ELASTICSEARCH_INDEX_POSTS` | ES index name for posts (default: `posts`) |
| `ELASTICSEARCH_INDEX_USERS` | ES index name for users (default: `users`) |
| `KAFKA_BROKERS` | Comma-separated Kafka brokers |
| `REDIS_HOST` / `REDIS_PORT` | Redis connection for autocomplete sync |

## Health Check

```bash
# API
curl http://localhost:8080/health

# Cassandra
docker compose exec cassandra-1 cqlsh -e "SELECT now() FROM system.local"

# Elasticsearch
curl http://localhost:9200/_cluster/health

# Kafka topics
docker compose exec kafka kafka-topics --bootstrap-server localhost:29092 --list
```
