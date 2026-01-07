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
| `cassandra` | 9042 | Cassandra database |
| `api` | 8080 | Go API server (optional) |

## Initialize Database

After starting Cassandra, apply the schema:

```bash
# Wait for Cassandra to be ready (~30 seconds)
docker-compose exec cassandra cqlsh -e "DESCRIBE KEYSPACES"

# Apply schema
docker cp migrations/cassandra_schema.cql geoloc_cassandra:/tmp/
docker exec geoloc_cassandra cqlsh -f /tmp/cassandra_schema.cql

# Optional: Seed test data
docker cp migrations/seed_test_data.cql geoloc_cassandra:/tmp/
docker exec geoloc_cassandra cqlsh -f /tmp/seed_test_data.cql
```

## Development Mode

Run API locally with Docker Cassandra:

```bash
# Start only Cassandra
docker-compose up -d cassandra

# Run API with live reload
go run cmd/api/main.go
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
    - CASSANDRA_HOST=cassandra
    - CASSANDRA_KEYSPACE=geoloc
    - JWT_SECRET=your-secret-key
```

## Health Check

```bash
# API
curl http://localhost:8080/health

# Cassandra
docker-compose exec cassandra cqlsh -e "SELECT now() FROM system.local"
```
