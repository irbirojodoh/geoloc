# Environment Configuration

Configuration is done via environment variables. The API, indexer, and backfill tools load env files in this order:

1. `.env.{APP_ENV}` — defaults to `.env.development` when `APP_ENV` is unset
2. `.env` — fallback (used by Docker Compose `env_file`)

For local `go run` development, put settings in **`.env.development`**. For Docker Compose services (`api`, `search-indexer`), use **`.env`** or `docker-compose.yml` `environment:` overrides.

Copy `.env.example` as a starting point.

## Required Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CASSANDRA_HOST` | Cassandra host address | `localhost` |
| `CASSANDRA_KEYSPACE` | Keyspace name | `geoloc` |
| `JWT_SECRET` | Secret key for JWT signing | *required* |
| `PORT` | API server port | `8080` |

## Optional — Core

| Variable | Description | Default |
|----------|-------------|---------|
| `CASSANDRA_PORT` | Cassandra CQL port | `9042` |
| `REDIS_HOST` | Redis host | `localhost` |
| `REDIS_PORT` | Redis port | `6379` |
| `BASE_URL` | Base URL for generated links | `http://localhost:8080` |
| `GIN_MODE` | Gin framework mode | `debug` |
| `ALLOWED_ORIGINS` | CORS origins (comma-separated) | `http://localhost:3000` |
| `APP_ENV` | Environment name (`development`, `staging`, `production`) | `development` |

## Storage (Cloudflare R2)

Media uploads (avatars, cover images, post images) are stored in Cloudflare R2. The bucket should remain **private** (disable Public Access in the Cloudflare dashboard).

API responses resolve stored object keys to **presigned GET URLs** (15-minute TTL). Clients download media **directly from R2**; the Go API does not proxy read traffic. Uploads use either server-side multipart endpoints or presigned PUT URLs.

When `R2_ACCOUNT_ID` is not set, the API logs a warning and the fallback media store returns errors for upload/presign operations.

| Variable | Required | Description |
|----------|----------|-------------|
| `R2_ACCOUNT_ID` | Yes (prod) | Cloudflare account ID |
| `R2_ACCESS_KEY_ID` | Yes (prod) | R2 API token access key ID |
| `R2_SECRET_ACCESS_KEY` | Yes (prod) | R2 API token secret |
| `R2_BUCKET_NAME` | No | Bucket name (default: `geoloc-media`) |

`R2_PUBLIC_DOMAIN` and `BASE_URL` are **not** used for media URL resolution. `BASE_URL` remains available for general deployment configuration and client documentation.

**Example (production):**

```env
R2_ACCOUNT_ID=your-account-id
R2_ACCESS_KEY_ID=your-access-key
R2_SECRET_ACCESS_KEY=your-secret-key
R2_BUCKET_NAME=geoloc-media
BASE_URL=https://api.yourdomain.com
```

Apply migration `migrations/009_cover_image_url.cql` before using cover image uploads.

## Optional — Kafka

| Variable | Description | Default |
|----------|-------------|---------|
| `KAFKA_BROKERS` | Comma-separated broker addresses | — |
| `KAFKA_NOTIFICATIONS_ENABLED` | Enable notification Kafka producers/consumers in API | `false` |
| `KAFKA_CONSUMER_GROUP_PREFIX` | Prefix for notification consumer groups | `geoloc` |

**Local development:** use `127.0.0.1:9092` (not `localhost`) to avoid IPv6 loopback issues with Docker on macOS.

**Docker Compose:** use `kafka:29092` (internal listener).

Setting `KAFKA_BROKERS` also enables the API's `posts.created` search-indexing producer.

## Optional — Elasticsearch Search

| Variable | Description | Default |
|----------|-------------|---------|
| `ELASTICSEARCH_URL` | Elasticsearch HTTP endpoint | `http://localhost:9200` |
| `ELASTICSEARCH_INDEX_POSTS` | Posts index name | `posts` |
| `ELASTICSEARCH_INDEX_USERS` | Users index name | `users` |
| `SEARCH_MAX_RESULTS` | Max ES hits per query | `20` |
| `SEARCH_DEFAULT_RADIUS_KM` | Default geo search radius | `5` |

## Example `.env.development` (local `go run`)

```env
# Database
CASSANDRA_HOST=localhost
CASSANDRA_KEYSPACE=geoloc

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379

# Security
JWT_SECRET=your-super-secret-key-here

# Server
PORT=8080
BASE_URL=http://localhost:8080

# Kafka (search indexing + notifications)
KAFKA_BROKERS=127.0.0.1:9092
KAFKA_NOTIFICATIONS_ENABLED=true

# Elasticsearch
ELASTICSEARCH_URL=http://localhost:9200
ELASTICSEARCH_INDEX_POSTS=posts
ELASTICSEARCH_INDEX_USERS=users
```

## Production Recommendations

1. **JWT_SECRET**: Use a strong, randomly generated secret (32+ characters)
2. **GIN_MODE**: Set to `release` for production
3. **CASSANDRA_HOST**: Use your production Cassandra cluster address
4. **BASE_URL**: Set to your production domain (e.g., `https://api.yourapp.com`)
5. **KAFKA_BROKERS**: Use your managed Kafka cluster endpoints
6. **ELASTICSEARCH_URL**: Use your production ES cluster URL with TLS/auth as required

## Optional — Push notifications (FCM)

| Variable | Description | Default |
|----------|-------------|---------|
| `PUSH_NOTIFICATIONS_ENABLED` | Use Firebase Admin SDK instead of log-only push | `false` |
| `FCM_PROJECT_ID` | Firebase project ID | — |
| `FCM_CREDENTIALS_JSON` | Service account JSON (single line) or path via env | — |

Requires migration `migrations/006_notifications_v2.cql` (`push_device_tokens` table).

**Local example** (do not commit real credentials):

```env
PUSH_NOTIFICATIONS_ENABLED=true
FCM_PROJECT_ID=your-firebase-project-id
FCM_CREDENTIALS_JSON={"type":"service_account",...}
```

See [Push notification testing](./testing-push-notifications.md).
