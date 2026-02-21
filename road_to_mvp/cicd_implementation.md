# DevOps & Infrastructure — Implementation Guide

> **Context:** This guide details all DevOps and infrastructure work required before Beta launch, as identified in [beta_progress.md](./beta_progress.md) Section 4.
>
> **Total Estimated Effort:** ~8 hours

---

## 1. Dockerfile for Go API

**Priority:** 🔴 P0 | **Effort:** ~1 hour

The project currently has no Dockerfile. The Go API cannot be containerized or deployed.

### What to Create

```
[NEW] Dockerfile
[NEW] .dockerignore
```

### Dockerfile Specification

```dockerfile
# --- Build Stage ---
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /geoloc-api ./cmd/api

# --- Runtime Stage ---
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

# Non-root user
RUN adduser -D -u 1001 appuser
USER appuser

WORKDIR /app
COPY --from=builder /geoloc-api .

# Create uploads directory
RUN mkdir -p /app/uploads

EXPOSE 8080

ENTRYPOINT ["./geoloc-api"]
```

### .dockerignore

```
.git
.env
uploads/
*.md
docs/
migrations/
internal/**/*_test.go
```

### Add API Service to docker-compose.yml

```yaml
  api:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: geoloc-api
    ports:
      - "8080:8080"
    env_file:
      - .env
    depends_on:
      cassandra-db-init:
        condition: service_completed_successfully
      redis:
        condition: service_started
    volumes:
      - ./uploads:/app/uploads
    restart: unless-stopped
```

---

## 2. CI/CD Pipeline (GitHub Actions)

**Priority:** 🔴 P0 | **Effort:** ~2 hours

No CI/CD pipeline exists. Tests and builds are entirely manual.

### What to Create

```
[NEW] .github/workflows/ci.yml        — runs on every push/PR
[NEW] .github/workflows/deploy.yml     — runs on push to main (optional)
```

### CI Pipeline (`ci.yml`)

**Triggers:** Push to any branch, Pull Requests to `main`

**Jobs:**

| Job | Steps | Purpose |
|---|---|---|
| **lint** | `golangci-lint run` | Static analysis & style |
| **test** | `go test -v -count=1 -race ./internal/auth/...` | Unit tests (no containers needed) |
| **integration** | Docker Compose + `go test ./internal/data/... ./internal/handlers/...` | Integration tests with Cassandra |
| **build** | `docker build .` | Verify the image builds |

```yaml
name: CI

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.24"
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v4

  test-unit:
    runs-on: ubuntu-latest
    env:
      JWT_SECRET: ci-test-secret
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.24"
      - run: go test -v -count=1 -race ./internal/auth/...

  test-integration:
    runs-on: ubuntu-latest
    env:
      JWT_SECRET: ci-test-secret
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.24"
      - name: Run integration tests (testcontainers)
        run: go test -v -count=1 -timeout 300s ./internal/data/... ./internal/handlers/...

  build:
    runs-on: ubuntu-latest
    needs: [lint, test-unit]
    steps:
      - uses: actions/checkout@v4
      - name: Build Docker image
        run: docker build -t geoloc-api:${{ github.sha }} .
```

### Deploy Pipeline (optional, `deploy.yml`)

Only needed if deploying to a cloud environment. Typical targets:

- **AWS ECS/Fargate** — push image to ECR, update service
- **Google Cloud Run** — push to Artifact Registry, deploy
- **VPS with Docker** — SSH + `docker compose pull && docker compose up -d`

This can be added later based on your chosen hosting platform.

---

## 3. Environment Separation

**Priority:** 🟡 P1 | **Effort:** ~30 min

Currently a single `.env` file is used for all environments.

### What to Create

```
[NEW] .env.development
[NEW] .env.staging
[NEW] .env.production
```

### Key Differences by Environment

| Variable | Development | Staging | Production |
|---|---|---|---|
| `GIN_MODE` | `debug` | `release` | `release` |
| `CASSANDRA_HOSTS` | `localhost` | staging host | prod cluster |
| `CASSANDRA_REPLICATION` | `1` | `2` | `3` |
| `ALLOWED_ORIGINS` | `http://localhost:3000` | staging domain | production domain |
| `JWT_SECRET` | dev secret | unique per env | unique, rotated |
| `LOG_LEVEL` | `debug` | `info` | `warn` |

### Load Order

Update `main.go` to support environment-specific files:

```go
env := os.Getenv("APP_ENV") // "development", "staging", "production"
if env == "" {
    env = "development"
}
godotenv.Load(".env." + env)  // Load env-specific first
godotenv.Load()                // Then fallback to .env
```

---

## 4. Health Check Improvements

**Priority:** 🟡 P1 | **Effort:** ~30 min

Current health check only pings Cassandra. It should also check Redis.

### Update `/health` Endpoint

```go
router.GET("/health", func(c *gin.Context) {
    health := gin.H{"status": "ok"}

    // Check Cassandra
    if err := session.Query("SELECT now() FROM system.local").Exec(); err != nil {
        health["status"] = "degraded"
        health["cassandra"] = "unhealthy"
    } else {
        health["cassandra"] = "ok"
    }

    // Check Redis
    if redisClient != nil {
        if err := redisClient.Ping(c.Request.Context()).Err(); err != nil {
            health["status"] = "degraded"
            health["redis"] = "unhealthy"
        } else {
            health["redis"] = "ok"
        }
    }

    status := http.StatusOK
    if health["status"] == "degraded" {
        status = http.StatusServiceUnavailable
    }
    c.JSON(status, health)
})
```

---

## 5. TLS / HTTPS

**Priority:** 🟡 P1 | **Effort:** ~1 hour

No SSL/TLS is configured. All traffic is plaintext HTTP.

### Options

| Approach | When to Use | Effort |
|---|---|---|
| **Reverse proxy (recommended)** | Production behind Nginx/Caddy/Traefik | 30 min |
| **Cloud load balancer** | AWS ALB, GCP Load Balancer | 15 min |
| **In-app TLS** | Standalone deployment | 1 hour |

### Recommended: Caddy Reverse Proxy

Add to `docker-compose.yml`:

```yaml
  caddy:
    image: caddy:2-alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - caddy_data:/data
    depends_on:
      - api
```

Create `Caddyfile`:

```
your-domain.com {
    reverse_proxy api:8080
}
```

Caddy automatically provisions and renews Let's Encrypt certificates.

---

## 6. Monitoring & Logging

**Priority:** 🟡 P2 (post-beta acceptable) | **Effort:** ~2 hours

Currently only `slog` to stdout with no aggregation or metrics.

### Minimum Viable Monitoring

| Layer | Tool | Purpose |
|---|---|---|
| **Structured Logs** | `slog` → stdout → Docker logs | Already in place ✅ |
| **Log Aggregation** | Loki + Grafana _or_ CloudWatch | Centralized log search |
| **Metrics** | Prometheus + `gin-contrib/prometheus` | Request rate, latency, error rate |
| **Uptime** | UptimeRobot / Betteruptime | External health check alerts |
| **Error Tracking** | Sentry Go SDK | Crash reporting with stack traces |

### Quick Win: Prometheus Metrics Middleware

```go
import "github.com/zsais/go-gin-prometheus"

p := ginprometheus.NewPrometheus("gin")
p.Use(router)
```

This exposes `/metrics` for Prometheus to scrape.

---

## 7. Makefile Expansion

**Priority:** 🟡 P2 | **Effort:** ~15 min

Current Makefile only has `run`, `test`, `docker-up`.

### Recommended Targets

```makefile
.PHONY: run test test-auth test-data build docker-up docker-down lint

run:
	go run cmd/api/main.go

build:
	docker build -t geoloc-api:latest .

test:
	JWT_SECRET=test-secret go test -v -count=1 ./...

test-auth:
	JWT_SECRET=test-secret go test -v -count=1 ./internal/auth/...

test-data:
	JWT_SECRET=test-secret go test -v -count=1 ./internal/data/...

test-handlers:
	JWT_SECRET=test-secret go test -v -count=1 ./internal/handlers/...

lint:
	golangci-lint run ./...

docker-up:
	docker compose down -v
	docker compose up -d

docker-down:
	docker compose down

check-cassandra-nodes:
	docker exec -it cassandra-1 nodetool status

logs:
	docker compose logs -f api
```

---

## Implementation Order

| Step | Item | Depends On | Est. Time |
|---|---|---|---|
| 1 | Dockerfile + .dockerignore | — | 1h |
| 2 | Add API to docker-compose.yml | Step 1 | 15min |
| 3 | GitHub Actions CI pipeline | Step 1 | 2h |
| 4 | Environment separation (.env files) | — | 30min |
| 5 | Health check improvements | — | 30min |
| 6 | TLS via Caddy reverse proxy | Step 2 | 1h |
| 7 | Expand Makefile | — | 15min |
| 8 | Prometheus metrics _(optional)_ | — | 1h |
| **Total** | | | **~6.5h** |

---

> **Start with items 1–3** (Dockerfile + CI). These unblock everything else — you can't deploy, test automatically, or add TLS without a container image and a pipeline.
