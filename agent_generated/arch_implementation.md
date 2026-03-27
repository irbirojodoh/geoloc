# Technical Architecture — Implementation Plan

This document outlines the implementation strategy to resolve the technical debt and architectural risks identified in Section 2 of the beta readiness report.

## Proposed Changes

### Database — Cassandra Configuration & Scalability

#### [MODIFY] [cassandra_schema.cql](../../migrations/cassandra_schema.cql)
- Update `CREATE KEYSPACE` to use `NetworkTopologyStrategy` instead of `SimpleStrategy` with a `replication_factor` of 3 for the production environment, eliminating the single point of failure risk. Right now it's hardcoded to `SimpleStrategy, 1`.

#### [MODIFY] [user_repo.go (Search)](../../internal/data/user_repo.go)
- **Replace `ALLOW FILTERING` in Search**: Because setting up Elasticsearch is heavy for an MVP, and Cassandra 4.1 supports Storage-Attached Indexes (SAI), we will create SAI indexes on `username` and `full_name` fields in `users` table via `cassandra_schema.cql`, and refactor `SearchUsers` to query the index instead of performing full-table scans. 
- *Note:* We will apply this to `SearchPosts` as well if `ALLOW FILTERING` is used there.

#### [MODIFY] [main.go (Cassandra Pool)](../../cmd/api/main.go)
- Configure gocql Connection Pooling explicitly:
  - `cluster.NumConns = 4` (connections per host)
  - `cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(gocql.RoundRobinHostPolicy())`

---

### API Gateway — Rate Limiting & Resilience 

#### [MODIFY] [ratelimit.go](../../internal/middleware/ratelimit.go)
- **Migrate to Redis**: The current `middleware.RateLimitByIP` maps IPs in-memory with a `sync.RWMutex`. This breaks in a multi-container deployment.
- Rewrite the `RateLimiter` to use the existing Redis connection (which is currently just used for likes) utilizing a sliding-window or fixed-window Redis algorithm (e.g. `INCR` + `EXPIRE`).

#### [MODIFY] [timeout.go (New File)](../../internal/middleware/timeout.go)
- **Request Timeout Middleware**: Add a new Gin middleware wrapped around `http.TimeoutHandler` or using context cancellation to enforce a global API timeout (e.g., 10 seconds), preventing slow external APIs (like Nominatim) from exhausting server goroutines.

#### [MODIFY] [main.go (Graceful Shutdown)](../../cmd/api/main.go)
- **Implement Graceful Shutdown**: Replace the default `router.Run()` with a custom `http.Server` running in a goroutine. Listen for `SIGINT` and `SIGTERM` signals. When caught, call `srv.Shutdown(ctx)` with a 5-second deadline to allow in-flight requests (like uploads or payment hooks) to finish cleanly.

---

### Concurrency & External Integrations

#### [MODIFY] [like_repo.go](../../internal/data/like_repo.go)
- **Background Goroutine Contexts**: Fix detached `go r.insertLegacyLike(context.Background(), ...)` instances. Create a proper background job worker or ensure the parent request context isn't bypassed without proper panic recovery and error logging bounds. 

#### [MODIFY] [nominatim.go](../../internal/geocoding/nominatim.go)
- **Single-threaded Bottleneck**: Currently uses a global mutex `sync.Mutex` and a ticker. To respect Nominatim's strict 1 rps policy without freezing the entire backend, implement an asynchronous geocoding worker pool or fallback to a caching layer so requests don't queue indefinitely.

## Verification Plan

### Automated Tests
- Run `make test-unit` to test the new Redis-backed rate limiter (mocked via miniredis).
- Run `make test-integration` to confirm the Cassandra connection pooling and SAI index searches still return correctly structured data.
- Run `make test-e2e` to ensure the timeout middleware and graceful shutdown logic do not break existing HTTP flows.

### Manual Verification
- Simulate multi-container rate limiting by spinning up two API instances on different ports and hitting them rapidly; confirm Redis successfully blocks across both.
- Send a `SIGTERM` to the server during a long artificial sleep request to visually verify the server waits for the request to complete before exiting.
