# Quick Start Guide

Get Geoloc running locally in under 5 minutes.

## Prerequisites

- Go 1.21+
- Docker & Docker Compose
- Git

## 1. Clone & Setup

```bash
git clone <repository-url>
cd geoloc

# Copy environment file
cp .env.example .env
```

## 2. Start Database

```bash
docker-compose up -d cassandra
```

Wait ~30 seconds for Cassandra to initialize, then apply the schema:

```bash
docker cp migrations/cassandra_schema.cql geoloc_cassandra:/tmp/
docker exec geoloc_cassandra cqlsh -f /tmp/cassandra_schema.cql
```

## 3. Run the API

```bash
go run cmd/api/main.go
```

The API will be available at `http://localhost:8080`.

## 4. Verify

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
```

## 5. Optional: Seed Test Data

```bash
docker cp migrations/seed_test_data.cql geoloc_cassandra:/tmp/
docker exec geoloc_cassandra cqlsh -f /tmp/seed_test_data.cql
```

---

## Next Steps

- [Environment Configuration](./environment.md) - Configure environment variables
- [API Overview](./api/README.md) - Learn about the API
- [Flutter Client Guide](./client/flutter.md) - Build the mobile app
