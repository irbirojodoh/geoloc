# Comprehensive Go Backend Analysis — `social-geo-go`

## Executive Summary

This is a **mid-maturity, feature-rich** geo-social backend built with Go 1.24, Gin, Apache Cassandra, and Redis. The project demonstrates strong fundamentals: proper `cmd/`+`internal/` layout, API versioning (`/api/v1`), JWT auth with refresh tokens, graceful shutdown, multi-stage Docker builds, a 3-node Cassandra cluster with Caddy TLS proxy, and an impressive E2E test suite powered by Testcontainers. The denormalized Cassandra data model is well thought-out with geohash-based partitioning, batched writes, and LWT-based idempotent likes.

**Main Concerns:** A critical password hashing vulnerability (SHA-based, no salt/cost factor), heavy use of `ALLOW FILTERING` in Cassandra queries, hardcoded secrets in [.env](file:///Users/rijal/Documents/Projects/geoloc/.env), absence of interfaces in the data layer preventing mockability, a race-condition-prone timeout middleware, and no CI/CD pipeline. With targeted fixes—especially around security—this codebase could be production-ready.

---

## Findings by Dimension

### 1. Project Structure & Architecture

**Current State:** The project follows a simplified **repository pattern** with a clear separation:

```
cmd/api/main.go          → Entry point, wiring, server lifecycle
internal/auth/           → JWT, OAuth (Goth), password, middleware
internal/cache/          → Redis client + LikeCounter
internal/data/           → Cassandra repositories + models
internal/geocoding/      → Nominatim reverse geocoding client
internal/handlers/       → Gin HTTP handlers (all route logic)
internal/middleware/     → Rate limiting, timeout
internal/push/           → Push notification interface + log stub
internal/storage/        → File upload interface (local + S3 stub)
migrations/              → Cassandra CQL schema + seed data
```

| Issue | Severity | Details |
|-------|----------|---------|
| No `pkg/` for shared types | **LOW** | Acceptable — everything is internal |
| [main.go](file:///Users/rijal/Documents/Projects/geoloc/cmd/api/main.go) is a monolith wiring file (283 lines) | **LOW** | Consider extracting server setup into `internal/server/` |
| No circular dependencies detected | ✅ | Clean unidirectional dependency flow |
| Missing `internal/service/` layer | **MEDIUM** | Handlers call repos directly — business logic is mixed in handlers |

### 2. Code Quality & Go Idioms

**Current State:** Generally good Go style. Uses `context.Context` throughout, proper `defer` for cleanup, `slog` structured logging, and idiomatic error handling in most places.

| Issue | Severity | Details |
|-------|----------|---------|
| No interfaces at consumer side for repos | **HIGH** | [user_repo.go](file:///Users/rijal/Documents/Projects/geoloc/internal/data/user_repo.go), [post_repo.go](file:///Users/rijal/Documents/Projects/geoloc/internal/data/post_repo.go) — handlers accept `*data.PostRepository` (concrete type) instead of interfaces. Makes mocking impossible without Testcontainers |
| `fmt.Printf` for warnings instead of `slog` | **LOW** | [like_repo.go:88](file:///Users/rijal/Documents/Projects/geoloc/internal/data/like_repo.go#L88), [follow_repo.go:70](file:///Users/rijal/Documents/Projects/geoloc/internal/data/follow_repo.go#L70) — `fmt.Printf("WARNING:...")` bypasses structured logging |
| Race condition in timeout middleware | **HIGH** | [timeout.go:25-28](file:///Users/rijal/Documents/Projects/geoloc/internal/middleware/timeout.go#L25-L28) — `c.Next()` in goroutine, but Gin's `ResponseWriter` isn't goroutine-safe. Both the timeout handler and the actual handler may write to the response simultaneously |
| `LogPushService.tokens` is not thread-safe | **MEDIUM** | [push.go:24](file:///Users/rijal/Documents/Projects/geoloc/internal/push/push.go#L24) — `map[string][]DeviceToken` with no sync protection; concurrent calls to [RegisterDevice](file:///Users/rijal/Documents/Projects/geoloc/internal/push/push.go#34-43)/[UnregisterDevice](file:///Users/rijal/Documents/Projects/geoloc/internal/push/push.go#44-55) will cause a data race |
| Goroutine in login handler uses expired context | **MEDIUM** | [handlers/auth.go:150](file:///Users/rijal/Documents/Projects/geoloc/internal/handlers/auth.go#L150) — `go userRepo.UpdateLastSeen(c.Request.Context(), ...)` passes request context to a background goroutine; the context will cancel as soon as the request finishes |
| [getEnv](file:///Users/rijal/Documents/Projects/geoloc/cmd/api/main.go#276-283) helper duplicated | **LOW** | Exists in both [cmd/api/main.go:276](file:///Users/rijal/Documents/Projects/geoloc/cmd/api/main.go#L276) and [cache/redis.go:67](file:///Users/rijal/Documents/Projects/geoloc/internal/cache/redis.go#L67) |
| Proper use of `errors.Is()` for token errors | ✅ | [auth_test.go:109](file:///Users/rijal/Documents/Projects/geoloc/internal/auth/auth_test.go#L109) |

### 3. API Design

**Current State:** Uses **Gin** framework with clean route organization. API versioning under `/api/v1`. Public routes for auth, protected routes under middleware group. Input validation via Gin's `binding` tags.

| Issue | Severity | Details |
|-------|----------|---------|
| Validation error messages are generic | **MEDIUM** | Handlers always return `"Invalid request body"` without specifying which field failed. Should parse `validator.ValidationErrors` |
| No request ID / correlation ID in responses | **LOW** | Makes debugging harder in production |
| Inconsistent error response schema | **MEDIUM** | Some responses use `{"error": "..."}`, others `{"error": "...", "details": "..."}`. No centralized error mapper |
| Auth routes not under `/api/v1` | **LOW** | `/auth/register`, `/auth/login` are outside the versioned group |
| [CreatePost](file:///Users/rijal/Documents/Projects/geoloc/internal/handlers/post.go#15-77) doesn't validate user owns the `user_id` | **HIGH** | [handlers/post.go:51](file:///Users/rijal/Documents/Projects/geoloc/internal/handlers/post.go#L51) — Verifies user exists but doesn't check that the authenticated user matches `req.UserID`. Any logged-in user can create posts as another user |

### 4. Data Layer

**Current State:** Uses **Apache Cassandra** (gocql) with a well-designed denormalized schema. Redis for like counters and rate limiting. Proper use of batched writes (logged batches), counter tables, LWT for idempotent likes.

| Issue | Severity | Details |
|-------|----------|---------|
| `ALLOW FILTERING` in user queries | **CRITICAL** | [user_repo.go:199](file:///Users/rijal/Documents/Projects/geoloc/internal/data/user_repo.go#L199), [user_repo.go:229](file:///Users/rijal/Documents/Projects/geoloc/internal/data/user_repo.go#L229) — [GetUserByUsername](file:///Users/rijal/Documents/Projects/geoloc/internal/data/user_repo.go#190-219) and [GetUserByEmail](file:///Users/rijal/Documents/Projects/geoloc/internal/data/user_repo.go#220-249) use `ALLOW FILTERING`. Despite having SAI indexes in the schema, the Go code still uses `ALLOW FILTERING` which causes full table scans |
| `ALLOW FILTERING` in location follows | **HIGH** | [location_follow_query.go:90](file:///Users/rijal/Documents/Projects/geoloc/internal/data/location_follow_query.go#L90) — [GetUsersFollowingLocation](file:///Users/rijal/Documents/Projects/geoloc/internal/data/location_follow_query.go#87-106) does full-table scan |
| Client-side search filtering | **HIGH** | [post_repo.go:244-268](file:///Users/rijal/Documents/Projects/geoloc/internal/data/post_repo.go#L244-L268) — [SearchPosts](file:///Users/rijal/Documents/Projects/geoloc/internal/data/post_repo.go#237-273) loads `limit*5` rows and filters in Go. Will degrade rapidly at scale |
| [GetUsersByIDs](file:///Users/rijal/Documents/Projects/geoloc/internal/data/user_repo.go#104-129) is N+1 query | **MEDIUM** | [user_repo.go:108-127](file:///Users/rijal/Documents/Projects/geoloc/internal/data/user_repo.go#L108-L127) — Loops through user IDs one-by-one. Should use `IN` clause or async queries |
| Connection pooling configured ✅ | ✅ | `NumConns: 4`, `TokenAwareHostPolicy`, Redis pool `PoolSize: 50` |
| Batched writes with context ✅ | ✅ | All multi-table writes use `gocql.LoggedBatch` |
| Good use of counter batches for follow counts ✅ | ✅ | [follow_repo.go:56](file:///Users/rijal/Documents/Projects/geoloc/internal/data/follow_repo.go#L56) |

### 5. Error Handling

**Current State:** Errors are consistently wrapped with `fmt.Errorf("...: %w", err)`. Sentinel errors defined for auth. However, some errors are silently discarded.

| Issue | Severity | Details |
|-------|----------|---------|
| Silenced errors in enrichment logic | **MEDIUM** | [handlers/post.go:177](file:///Users/rijal/Documents/Projects/geoloc/internal/handlers/post.go#L177) — `userInfoMap, _ := userRepo.GetUsersByIDs(...)` — errors from user enrichment are completely ignored; degraded UX without logging |
| No custom error types | **LOW** | Only auth has sentinel errors. Data layer uses string matching: `strings.Contains(err.Error(), "not found")` at [handlers/post.go:225](file:///Users/rijal/Documents/Projects/geoloc/internal/handlers/post.go#L225) — fragile pattern |
| Inconsistent error wrapping prefix | **LOW** | Mix of `"ERROR: failed to..."`, `"failed to..."`, `"invalid..."` prefixes |

### 6. Configuration & Secrets Management

**Current State:** Uses `godotenv` with environment-specific files ([.env.development](file:///Users/rijal/Documents/Projects/geoloc/.env.development), [.env.staging](file:///Users/rijal/Documents/Projects/geoloc/.env.staging), [.env.production](file:///Users/rijal/Documents/Projects/geoloc/.env.production)). Proper [.gitignore](file:///Users/rijal/Documents/Projects/geoloc/.gitignore) excluding [.env](file:///Users/rijal/Documents/Projects/geoloc/.env).

| Issue | Severity | Details |
|-------|----------|---------|
| [.env](file:///Users/rijal/Documents/Projects/geoloc/.env) committed with real secrets | **CRITICAL** | [.env:15](file:///Users/rijal/Documents/Projects/geoloc/.env#L15) — `SESSION_SECRET=AWDF4egMp/BMzjrQ...` is a real base64 secret checked into the repo. [.env](file:///Users/rijal/Documents/Projects/geoloc/.env) is in [.gitignore](file:///Users/rijal/Documents/Projects/geoloc/.gitignore) but the file currently exists with secrets |
| Default JWT secret hardcoded | **HIGH** | [jwt.go:87](file:///Users/rijal/Documents/Projects/geoloc/internal/auth/jwt.go#L87) — Falls back to `"your-super-secret-key-change-in-production"` if env var is missing |
| JWT_SECRET in [.env](file:///Users/rijal/Documents/Projects/geoloc/.env) is placeholder | **MEDIUM** | [.env:28](file:///Users/rijal/Documents/Projects/geoloc/.env#L28) — `JWT_SECRET=your-super-secret-jwt-key-here` |
| Good: env-specific config files | ✅ | [.env.development](file:///Users/rijal/Documents/Projects/geoloc/.env.development), [.env.staging](file:///Users/rijal/Documents/Projects/geoloc/.env.staging), [.env.production](file:///Users/rijal/Documents/Projects/geoloc/.env.production) with appropriate separation |

### 7. Testing

**Current State:** Excellent test infrastructure using **Testcontainers** for integration/E2E tests against real Cassandra. ~800 lines of E2E tests covering auth, posts, comments, likes, follows, notifications, location follows, and error leak prevention. Unit tests for auth package.

| Issue | Severity | Details |
|-------|----------|---------|
| No unit tests for handlers (only E2E) | **MEDIUM** | Without repo interfaces, handlers can only be tested via full E2E with Testcontainers (slow) |
| No test coverage reporting configured | **LOW** | Makefile `test` doesn't include `-cover` flag |
| [TestMain](file:///Users/rijal/Documents/Projects/geoloc/internal/data/setup_test.go#18-70) uses `os.Setenv` (non-parallel-safe) | **LOW** | [auth_test.go:19](file:///Users/rijal/Documents/Projects/geoloc/internal/auth/auth_test.go#L19) |
| Good: E2E error leak prevention test ✅ | ✅ | [e2e_test.go:773](file:///Users/rijal/Documents/Projects/geoloc/internal/handlers/e2e_test.go#L773) — validates no `details` key leaks in error responses |
| Good: Testcontainers with init scripts ✅ | ✅ | [setup_test.go](file:///Users/rijal/Documents/Projects/geoloc/internal/data/setup_test.go) uses `cassandra.WithInitScripts` |

### 8. Observability

**Current State:** Uses `slog` with `tint` handler for colored structured logging. Health endpoint checks both Cassandra and Redis.

| Issue | Severity | Details |
|-------|----------|---------|
| No metrics (Prometheus/OpenTelemetry) | **MEDIUM** | No request latency, error rate, or business metrics |
| No distributed tracing | **MEDIUM** | OpenTelemetry is in [go.mod](file:///Users/rijal/Documents/Projects/geoloc/go.mod) as indirect dep but not actively used |
| Mixed `log.Println` and `slog` usage | **LOW** | [main.go:72](file:///Users/rijal/Documents/Projects/geoloc/cmd/api/main.go#L72) uses `log.Printf`, while elsewhere `slog` is used |
| Health endpoint ✅ | ✅ | Checks Cassandra + Redis, returns degraded status |
| No readiness endpoint | **LOW** | Only `/health` exists, no separate `/ready` for k8s probes |

### 9. Security

**Current State:** JWT auth with access/refresh tokens, OAuth (Google + Apple), rate limiting, CORS configuration, non-root Docker user.

| Issue | Severity | Details |
|-------|----------|---------|
| **No bcrypt/argon2 for passwords** | **CRITICAL** | [password.go](file:///Users/rijal/Documents/Projects/geoloc/internal/auth/password.go) — Uses SHA3-512 → SHA-256 chain with **no salt and no cost factor**. Identical passwords produce identical hashes. Trivially rainbow-tabled. Must use `bcrypt` or `argon2id` |
| Password comparison is timing-attack vulnerable | **HIGH** | [password.go:27](file:///Users/rijal/Documents/Projects/geoloc/internal/auth/password.go#L27) — `computedHash == storedHash` should use `crypto/subtle.ConstantTimeCompare` |
| OAuth users have empty password hash | **MEDIUM** | [user_repo.go:80](file:///Users/rijal/Documents/Projects/geoloc/internal/data/user_repo.go#L80) — If password login is attempted for an OAuth-created user, [VerifyPassword("anything", "")](file:///Users/rijal/Documents/Projects/geoloc/internal/auth/password.go#24-29) will fail harmlessly, but the intent should be clearer |
| No token revocation/blacklist | **MEDIUM** | Compromised tokens remain valid until expiry (15 min access, 7 day refresh) |
| CreatePost allows user impersonation | **HIGH** | See API Design section |
| Good: rate limiting via Redis ✅ | ✅ | IP-based (100/min), with Redis backend for multi-instance |
| Good: CORS properly configured ✅ | ✅ | Configurable origins from env |
| Good: non-root Docker user ✅ | ✅ | `appuser` with UID 1001 |
|  `NominatimClient.rateLimiter` ticker never stopped | **LOW** | [nominatim.go:68](file:///Users/rijal/Documents/Projects/geoloc/internal/geocoding/nominatim.go#L68) — Ticker is created but never `Stop()`'d, leaking a goroutine |

### 10. Deployment Readiness

**Current State:** Multi-stage Dockerfile, 3-node Cassandra cluster with docker-compose, Caddy reverse proxy for TLS, graceful shutdown, schema init container.

| Issue | Severity | Details |
|-------|----------|---------|
| No CI/CD pipeline | **HIGH** | No `.github/workflows/*.yml` — no automated testing, linting, or deployment |
| No Kubernetes manifests | **LOW** | Acceptable if deploying via docker-compose initially |
| [docker-compose.yml](file:///Users/rijal/Documents/Projects/geoloc/docker-compose.yml) version field deprecated | **LOW** | `version: "3.8"` is deprecated in modern Docker Compose |
| Good: graceful shutdown ✅ | ✅ | Signal handling with 5s timeout context |
| Good: multi-stage Docker build ✅ | ✅ | Alpine builder + minimal runtime image, stripped binary |
| Good: health checks in compose ✅ | ✅ | All services have health checks with proper `depends_on` ordering |

### 11. Dependency Management

**Current State:** Go 1.24, well-maintained dependencies. Testcontainers for testing.

| Issue | Severity | Details |
|-------|----------|---------|
| Gin v1.9.1 is behind current v1.10+ | **LOW** | Minor — no known vulnerabilities |
| Leftover SQL migration files | **LOW** | `000001_init_schema.*.sql`, `000002_create_users_table.*.sql` — project uses Cassandra, not Postgres. These are stale |
| [go.mod](file:///Users/rijal/Documents/Projects/geoloc/go.mod) has stale `DB_*` env vars in [.env](file:///Users/rijal/Documents/Projects/geoloc/.env) | **LOW** | [.env:3-6](file:///Users/rijal/Documents/Projects/geoloc/.env#L3-L6) — references `DB_HOST`, `DB_PORT`, etc. for a Postgres that doesn't exist |
| Good: minimal direct dependencies ✅ | ✅ | Core: gin, gocql, go-redis, golang-jwt, goth, stretchr/testify |

---

## Top 5 Priority Improvements

### 1. 🔴 Replace SHA-based Password Hashing with bcrypt/argon2
> **Impact: Security — CRITICAL**
> The current SHA3→SHA256 chain is **unsalted** and **uncostable**, making all passwords trivially crackable. Switch to `golang.org/x/crypto/bcrypt` immediately. This is a must-fix before any production deployment.

### 2. 🔴 Remove `ALLOW FILTERING` from Cassandra Queries
> **Impact: Performance — CRITICAL at scale**
> [GetUserByUsername](file:///Users/rijal/Documents/Projects/geoloc/internal/data/user_repo.go#190-219) and [GetUserByEmail](file:///Users/rijal/Documents/Projects/geoloc/internal/data/user_repo.go#220-249) use `ALLOW FILTERING` despite having SAI indexes. Remove `ALLOW FILTERING` since the SAI indexes should handle the queries. For [GetUsersFollowingLocation](file:///Users/rijal/Documents/Projects/geoloc/internal/data/location_follow_query.go#87-106), create a reverse lookup table (`location_followers`).

### 3. 🟠 Introduce Repository Interfaces for Testability
> **Impact: Testing & Architecture — HIGH**
> Define interfaces (e.g., [UserRepository](file:///Users/rijal/Documents/Projects/geoloc/internal/data/user_repo.go#13-16), [PostRepository](file:///Users/rijal/Documents/Projects/geoloc/internal/data/post_repo.go#13-16)) at the consumer side (handlers package) or in a shared contract package. This enables unit testing handlers with mocks instead of requiring full Cassandra via Testcontainers.

### 4. 🟠 Fix User Impersonation in CreatePost
> **Impact: Security — HIGH**
> The [CreatePost](file:///Users/rijal/Documents/Projects/geoloc/internal/handlers/post.go#15-77) handler accepts `user_id` from the request body without verifying it matches the authenticated user. Replace `req.UserID` with `auth.GetUserID(c)` to prevent any user from posting as another.

### 5. 🟠 Add CI/CD Pipeline
> **Impact: Reliability — HIGH**
> Add a GitHub Actions workflow for linting (`golangci-lint`), unit tests, integration tests, Docker build verification, and dependency vulnerability scanning (`govulncheck`).

---

## Positive Highlights

| Area | Detail |
|------|--------|
| **Cassandra Schema Design** | Excellent denormalized model with geohash-partitioned posts, proper counter tables, LWT-based idempotent likes, and multi-table batched writes |
| **E2E Test Suite** | ~800 lines of comprehensive E2E tests using Testcontainers against real Cassandra — covers auth, CRUD, follows, likes, comments, notifications, and error leak prevention |
| **Graceful Degradation** | Redis is optional — like counts fall back to Cassandra, rate limiter fails-open, health endpoint reports degraded status |
| **Cursor-Based Pagination** | Proper base64-encoded timestamp cursors with `limit+1` pattern for `has_more` detection |
| **Docker & Deployment** | Multi-stage build, non-root user, 3-node Cassandra cluster, Caddy TLS proxy, schema init container, health checks throughout |
| **Rate Limiting** | Redis-backed rate limiter with IP and user-based variants, configurable window and limit |
| **OAuth Integration** | Full Google + Apple OAuth via Goth library with proper session management |
| **Location Caching** | Nominatim geocoding results cached in Cassandra with read-through pattern ([GetOrFetch](file:///Users/rijal/Documents/Projects/geoloc/internal/data/location_repo.go#87-129)) |
| **Like System Architecture** | Dual-layer design: Cassandra LWT for state (idempotent), Redis for counters (fast), with Lua scripts for atomic decrement-to-zero |
