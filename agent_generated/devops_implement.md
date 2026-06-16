# Geoloc Production-Readiness Assessment & Microservices Deployment Plan

This document provides a comprehensive production-readiness assessment of the Geoloc hyper-local social platform, along with a detailed architectural blueprint for migrating to a distributed, microservices-based topology.

---

## PART 1 — PRODUCTION-READINESS ASSESSMENT

### 1. Architecture & Service Boundaries

#### Current Monolith-ish Decomposition
The current codebase organizes the HTTP API (`cmd/api`), search-indexer (`cmd/indexer`), and one-off backfill jobs (`cmd/backfill-*`) into a single Git repository. While they are built into separate binaries, they share internal libraries and database configurations. 
*   **The Benefit**: Low deployment friction in development and consistent schemas.
*   **The Risk**: Shared codebase coupling makes it easy to bypass service boundaries and query other domains' databases directly.

#### In-Process Kafka Notifications Monolith (KAFKA_NOTIFICATIONS_ENABLED)
When `KAFKA_NOTIFICATIONS_ENABLED=true` is set, the API server starts four separate Kafka consumer groups in-process (`notif-persister`, `notif-push-dispatch`, `notif-push-retry`, `notif-nearby-fanout`). 
*   **Blast Radius & Coupling**: This is a major monolith risk. Running consumer groups in the web server process means CPU/Memory spikes during high notification traffic (e.g., a viral post causing thousands of nearby fan-outs) directly compete with the HTTP server's resources. A memory leak or panic in the `push-dispatch` consumer will crash the public REST API.
*   **Decoupling Strategy**: The notification pipeline must be extracted from `cmd/api`. The web API should strictly act as a producer. All consumer goroutines must be migrated to a dedicated, independently scaling worker pool (`notifications-service`).

#### Inter-Service Dependency Map

```
┌───────────┐      Sync / Blocking (Hard)     ┌───────────────┐
│  Go API   ├────────────────────────────────►│   Cassandra   │
└─────┬─────┘                                 └───────────────┘
      │
      │            Sync / Blocking (Hard)     ┌───────────────┐
      ├──────────────────────────────────────►│     Redis     │
      │                                       └───────────────┘
      │
      │            Sync / Blocking (Degraded) ┌───────────────┐
      ├──────────────────────────────────────►│   Nominatim   │
      │                                       └───────────────┘
      │
      │            Async / Non-Blocking (Soft)┌───────────────┐
      └───────────┐                           │ Kafka Broker  │
                  │                           └───────┬───────┘
                  ▼                                   │
          ┌───────────────┐                           │ Async (Soft)
          │  Indexer &    │◄──────────────────────────┘
          │  Consumers    │
          └───────┬───────┘
                  │
                  ├──────────────────────────────────► Elasticsearch (Async / Soft)
                  │
                  └──────────────────────────────────► FCM API (Async / Soft)
```

*   **API → Cassandra (Sync / Blocking / Hard)**: Core database connection. If Cassandra is down, the API cannot authenticate users, store posts, or fetch histories. Writes and reads block.
*   **API → Redis (Sync / Blocking / Hard)**: Required for rate-limiting, E2EE online presence checking, and atomic like/comment counters. If Redis fails, the system attempts to degrade to Cassandra counts, but this fallback introduces heavy query loads that can exhaust Cassandra (making it a *de facto* hard dependency).
*   **API → Nominatim (Sync / Blocking / Degradable)**: The API performs a blocking HTTP call during feed location enrichment if the geohash is not cached. If Nominatim fails, the API returns posts with empty location strings instead of crashing.
*   **API → Kafka Producer (Async / Non-Blocking / Degradable)**: Writing to Kafka is spawned in background goroutines in the handlers (`CreatePost`, `TogglePostLike`, `CreateComment`). If Kafka is down, the HTTP client gets a successful response, but the event is dropped, meaning search indexing and notifications fail.
*   **Indexer → Kafka Consumer → ES + Redis (Async / Non-Blocking / Degradable)**: The `search-indexer` reads events asynchronously. If Elasticsearch or Redis autocomplete is down, the indexer retries, backpressure builds in Kafka, and search results become stale.
*   **Kafka → FCM (Async / Non-Blocking / Degradable)**: The push dispatcher reads jobs from Kafka. If FCM is down, retry jobs are pushed to `notification.push.retry` for exponential backoff, isolating the core system from Firebase outages.

#### SSE Statefulness & Sticky Sessions
The `/api/v1/notifications/stream` endpoint establishes long-lived Server-Sent Events (SSE) connections. 
*   **Load Balancing**: Traditional round-robin load balancing will terminate connections prematurely. The edge proxy/load balancer must support long-lived TCP connections, disable response buffering, and manage TCP keep-alives. Sticky sessions are *not* strictly required because the real-time event distribution is backed by Redis Pub/Sub. When a user connects to *any* replica, the replica subscribes to Redis channel `sse:user:{userID}`. When an event is published to Redis, it is broadcast to whichever replica holds the active connection.
*   **Horizontal Scaling Constraints**: Scaling is limited by the maximum number of open file descriptors (`ulimit -n`) and memory per TCP connection. Reconnecting clients trigger "thundering herds"; clients must use exponential backoff with jitter.

---

### 2. Data Layer

#### Cassandra Production Topology
*   **Multi-AZ Topology**: A minimum of 3 nodes per Availability Zone (AZ) across 3 AZs (9 nodes total) is required. The cluster must use `GossipingPropertyFileSnitch` with AZs mapped as racks.
*   **Replication & Consistency**: Use `NetworkTopologyStrategy` with a replication factor of 3 per data center. 
    *   *Write Path*: Use `LOCAL_QUORUM` (requires acknowledgement from 2 nodes in the local DC) to ensure strong consistency without cross-region latency.
    *   *Read Path (Critical)*: Change the feed query path from `QUORUM` to `LOCAL_ONE` or `ONE`. High-frequency geolocation feed fetching does not require absolute real-time consistency. Using `LOCAL_ONE` improves read performance and keeps the system responsive if a node fails.
*   **Partition Key Hotspot Risk**: The `posts_by_geohash` table uses `geohash_prefix` (5-character geohash, ~5km) as the partition key. 
    *   *The Risk*: In dense metropolitan areas or during viral local events, a single geohash will receive thousands of posts and millions of queries, concentrating load on the single replica set holding that partition.
    *   *Remediation*: Sub-partition the geohash by adding a time bucket, e.g., `PRIMARY KEY ((geohash_prefix, date_bucket), created_at, post_id)` where `date_bucket` is the date (YYYY-MM-DD) or hour block.
*   **Backup & Restore**: Use Cassandra `nodetool snapshot` scheduled daily. Backups should be orchestrated and shipped to Cloudflare R2 using tools like **Medusa**.

#### Redis High Availability
*   **Failover Topology**: Deploy a Redis Cluster (3 shards, each with 1 replica) rather than a simple master-slave Sentinel setup to handle the high throughput of geoloc counters.
*   **Persistence Mode**: Enable AOF (Append Only File) with `appendfsync everysec` combined with RDB snapshots. Purely ephemeral caching is unacceptable because a Redis crash would lose active rate-limit tokens, user presence state, and atomic social counters.
*   **Eviction Policy**: Set `maxmemory-policy noeviction`. If Redis runs out of memory, it should return errors rather than evicting counter keys, which would cause counts to desynchronize from Cassandra.
*   **Pub/Sub Reliability**: Redis Pub/Sub offers **at-most-once** delivery. If a Redis node fails or a network partition occurs, active SSE clients will miss messages. SSE client code must query the fallback REST API (`GET /notifications` and `GET /dm/conversations/.../messages`) upon reconnection to fetch missed state.

#### Elasticsearch Operations
*   **Index Lifecycle Management (ILM)**: Set up an ILM policy to roll over the `posts` index when it reaches 50GB or 30 days, keeping search performance optimal.
*   **Reindexing Strategy**: Always query ES through an index alias (e.g., `posts-search` -> `posts-v1`). If a mapping change is required (such as altering the edge n-gram length), create a new index `posts-v2`, run a reindex task in the background, and flip the alias target atomically.
*   **Indexer Backpressure**: Kafka acts as a buffer. If Elasticsearch becomes slow, the indexer's consumer loop naturally slows down because the consumer blocks on indexing calls, preventing Elasticsearch from being overwhelmed.
*   **Consistency SLA**: Elasticsearch is eventually consistent. A post should appear in search results within **2 seconds** of creation.

#### Kafka Topic Configuration
*   **Topic Topology**:
    *   `posts.created`: 6 partitions, RF=3, `min.insync.replicas=2`, retention=7 days.
    *   `notification.events`: 12 partitions (higher partition density due to 1-to-N fan-out), RF=3, retention=3 days.
    *   `notification.push.dispatch`: 6 partitions, RF=3, retention=1 day.
*   **Consumer Rebalance**: Set `session.timeout.ms=45000` and `max.poll.interval.ms=300000` to prevent consumer groups from dropping out during heavy batch processing.
*   **Dead-Letter Queue (DLQ)**: If a consumer handler fails after 3 retries (e.g., due to a malformed payload), serialize the failed message along with error metadata and publish it to a `.dlq` topic (e.g., `notification.events.dlq`), then commit the offset to prevent queue stalling.

---

### 3. Security

#### Secrets Management
*   **Current State**: Secrets are loaded from plaintext `.env` files.
*   **Remediation**: Integrate the Kubernetes **External Secrets Operator (ESO)** with AWS Secrets Manager or HashiCorp Vault. Inject secrets into pods as environment variables or mounted files at runtime. Commit only template config files (e.g., `config.yaml`) to the repository.

#### AuthN/AuthZ Hardening
*   **Token Rotation**: Implement short-lived JWT access tokens (15 minutes) and longer-lived refresh tokens (7 days) stored in Cassandra. Revoked refresh tokens must be blacklisted in Redis with a TTL matching the token's expiration.
*   **OAuth Hardening**: The mobile OAuth endpoints (`/auth/google/token` and `/auth/apple/token`) must verify the cryptographic signatures of incoming ID tokens against Google/Apple public JWKS endpoints, and validate the `aud` (audience) and `iss` (issuer) fields.
*   **Bcrypt Cost**: Enforce a minimum bcrypt cost factor of `12` in production.

#### E2EE Direct Messages
*   **Cryptographic Assessment**: The E2EE design (X25519 for key exchange, HKDF-SHA256 for key derivation, and AES-256-GCM for encryption) is solid. The server only sees base64-encoded ciphertext, nonces, and public keys.
*   **Metadata Leakage**: Although message contents are secure, the server learns:
    1.  The social graph (who talks to whom, and when).
    2.  The approximate message size (which can leak information about language patterns or media sharing).
    3.  User online presence status.
*   **Remediation**: Implement strict log-redaction policies to ensure conversation participant IDs, IP addresses, and message sizes are not written to application logs.

#### Network Security
*   **Topology**: Place Cassandra, Redis, Kafka, and Elasticsearch in isolated private subnets. Only the API Gateway/Caddy proxy should be exposed to the public internet.
*   **Internal Communication**: Enforce mTLS between all microservices. Use Kubernetes **NetworkPolicies** to lock down ingress; for example, only allow the `search-indexer` namespace to access Elasticsearch.
*   **CORS**: Restrict `ALLOWED_ORIGINS` to the exact production web/mobile callback domains (never use `*`).

#### Geolocation Abuse Vectors & Input Validation
*   **Location Spoofing**: Clients can send fabricated GPS coordinates in API requests.
    *   *Mitigation*: Implement IP-to-location verification in the API middleware. If the distance between the client's IP location and their requested GPS coordinates exceeds a reasonable threshold (e.g., 500km), flag the request or fallback to IP-based geohashing.
*   **User Triangulation & Stalking**: Attackers can query nearby posts/users from three distinct offset points to triangulate a user's exact coordinates.
    *   *Mitigation*: Snapping query coordinates to a static grid or adding a small random fuzzy offset to geohashes returned in public queries. Limit nearby query frequency per user.

---

### 4. Reliability & Observability

#### Observability Gaps
The current system lacks structured metrics, tracing, and log aggregation.
*   **Logging**: Standardize on structured JSON logs outputted to `stdout` to be collected by **Loki** or **FluentBit**.
*   **Metrics**: Export Prometheus metrics (`/metrics`) tracking Go runtime stats, HTTP request rates/latencies (using Prometheus histogram buckets), DB query latencies, and Kafka consumer group lag.
*   **Tracing**: OpenTelemetry SDKs are integrated into Go API servers, Kafka consumers, and DB queries. Tracing spans are sent to **Tempo**.

#### Failure Modes
*   **Nominatim Outage**: The Nominatim client reverse-geolocates coordinates to names. If it goes down, the API continues serving posts with empty location fields, maintaining core availability.
*   **Kafka Outage**: Writing to Kafka occurs in background goroutines. If Kafka is down, post creation still succeeds, but search indexing and push notifications fail. This should be made more robust: if publishing to Kafka fails, the API should write the event to a local disk-backed buffer (e.g., a file-based queue) to be retried when Kafka recovers.
*   **Elasticsearch Outage**: The main write path (Cassandra) remains active. The search API degrades, returning a `503 Service Unavailable` or falling back to the legacy Cassandra database search.

#### Backfill Jobs
*   `backfill-search` and `backfill-comment-counts` are idempotent. They read from Cassandra and overwrite Elasticsearch/Redis. They are safe to run against a live system, but should be rate-limited to prevent performance degradation on Cassandra during business hours.

---

### 5. Testing & CI/CD Gaps

*   **Test Isolation**: The integration tests use `testcontainers` to launch Cassandra. While this provides great isolation, spinning up a full Cassandra container for each package test run makes the CI pipeline slow.
*   **Staging & Rollback**: There is no staging environment. Migrations are applied forward only.
    *   *Remediation*: Implement a dual-schema strategy (e.g., never run destructive `DROP` migrations; instead, deploy changes using additive schema updates so older code versions can run during a rollback).

---

### 6. Compliance & Data Governance

*   **GDPR / CCPA**: Geolocation coordinates are sensitive personal data. 
    *   *Erasure*: When a user deletes their account, a deletion event must be published to Kafka. All associated database records, search indices, and Redis caches must be purged.
    *   *Third-Party Data Leaks*: Querying the public Nominatim instance transmits user coordinates to an external service. This violates GDPR unless explicitly consented to. The production environment **must** use a private, self-hosted Nominatim/OSM instance.

---

## PRIORITIZED RISK REGISTER

| ID | Title | Priority | Current State | Risk | Business Impact | Remediation Owner |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **R-01** | Hot Cassandra Partitions | **Critical** | `posts_by_geohash` partitioned solely by `geohash_prefix` (5-char). | Dense locations or viral local events concentrate massive read/write traffic on a single Cassandra replica set. | Node instability, timeout errors, partition size limit exhaustion, system outage. | Backend / Platform |
| **R-02** | Redis Counter Failure Cascade | **Critical** | Likes/comments counts fall back to Cassandra `SELECT COUNT(*)` queries if Redis is offline. | Redis connection drops trigger heavy database table scans for every post returned in the feed. | Cassandra cluster exhaustion (DOS), complete backend failure. | Backend |
| **R-03** | In-Process Notification Monolith | **High** | Notification consumers run inside the main API server process. | Heavy push notification fan-outs consume HTTP server CPU/Memory. | REST API crashes or becomes unresponsive during high social activity. | Platform |
| **R-04** | Plaintext Secrets Management | **High** | Credentials and secrets are stored in `.env` files and environment variables. | Exposed container layers or memory dumps leak cloud credentials and JWT keys. | Complete system compromise, unauthorized database access. | Security / Platform |
| **R-05** | Geocoding Goroutine Starvation | **High** | Nominatim requests wait sequentially on a single 1.1s ticker. | Concurrent feed requests block waiting for the ticker, timing out after 10s. | Slow feed loading times, gateway timeouts, connection pool exhaustion. | Backend |
| **R-06** | Compliance Data Leak via Nominatim | **High** | Geocoding queries coordinates to a public OpenStreetMap instance. | Transmitting user coordinates to third-party public geocoders violates GDPR/CCPA. | Regulatory fines, compliance audits, user privacy leakage. | Security / Backend |
| **R-07** | E2EE DM Metadata Leakage | **Medium** | Server logs/retains conversation graphs, message sizes, and timings. | An attacker with database access can reconstruct user interaction graphs. | Loss of user trust, privacy compliance violations. | Security |
| **R-08** | Ephemeral SSE Pub/Sub | **Medium** | SSE uses Redis Pub/Sub with at-most-once delivery. | Messages sent during a client network drop are permanently lost. | Missed real-time DMs and notifications. | Backend |
| **R-09** | Lack of Observability Stack | **Medium** | No structured logging, metrics collection, or tracing. | Unable to trace requests or monitor Kafka consumer group lag. | High Mean Time to Repair (MTTR), hidden performance bottlenecks. | Platform |
| **R-10** | Slow CI Integration Tests | **Low** | Integration tests start a separate Cassandra container per package. | High test execution times delay developer feedback loop. | Slow development cycles, pipeline friction. | Platform |

---

## PART 2 — PRODUCTION DEPLOYMENT IMPLEMENTATION PLAN

### 1. Target Service Decomposition

To achieve independent scaling, fault isolation, and clear boundary separation, the monolith will be split into the following microservices:

```
                            ┌───────────────────┐
                            │    Mobile / Web   │
                            └─────────┬─────────┘
                                      │ HTTPS
                                      ▼
                            ┌───────────────────┐
                            │    API Gateway    │
                            │   (Envoy / Edge)  │
                            └────┬──┬───┬───┬───┘
         ┌───────────────────────┘  │   │   │   └───────────────────────┐
         ▼                          ▼   │   ▼                           ▼
┌─────────────────┐ ┌─────────────────┐ │ ┌─────────────────┐ ┌─────────────────┐
│  auth-service   │ │  users-service  │ │ │  posts-service  │ │   dm-service    │
└─────────────────┘ └─────────────────┘ │ └─────────────────┘ └─────────────────┘
                                        ▼
                             ┌──────────────────┐
                             │   feed-service   │
                             └──────────────────┘

============================= ASYNCHRONOUS LAYER =============================

                                ┌──────────────┐
                                │ Kafka Broker │
                                └──────┬───────┘
                                       │
                ┌──────────────────────┼──────────────────────┐
                ▼                      ▼                      ▼
      ┌──────────────────┐   ┌──────────────────┐   ┌──────────────────┐
      │  search-indexer  │   │notif-dispatcher  │   │notif-push-worker │
      └────────┬─────────┘   └────────┬─────────┘   └────────┬─────────┘
               ▼                      ▼                      ▼
      ┌──────────────────┐   ┌──────────────────┐   ┌──────────────────┐
      │  Elasticsearch   │   │    Cassandra     │   │     FCM API      │
      └──────────────────┘   └──────────────────┘   └──────────────────┘
```

#### Service Boundaries & Phase Isolation
1.  **api-gateway (Phase 1)**: Envoy or Caddy routing traffic to downstream services. Handles SSL termination and global rate-limiting.
2.  **auth-service (Phase 2)**: Manages registration, login, JWT issuance, and OAuth token verification.
3.  **users-service (Phase 2)**: Handles profile updates, search indices for users, and follower graphs.
4.  **posts-service (Phase 2)**: Post creation and database writes. Dispatches `posts.created` events to Kafka.
5.  **feed-service (Phase 2)**: Proximity calculations, neighboring geohash fetches, and feed compilation. Isolated from write paths.
6.  **dm-service (Phase 3)**: Manages public key registration, conversation metadata, and message routing. Isolated due to high compliance and encryption sensitivity.
7.  **search-indexer (Phase 1)**: Consumes Kafka events and writes to Elasticsearch. (Already structurally separated in the codebase, making it low risk).
8.  **notifications-service (Phase 1)**: Split into two worker deployments:
    *   `notif-dispatcher-worker`: Consumes `notification.events`, applies block/mute checks, and persists notifications to Cassandra.
    *   `notif-push-worker`: Consumes `notification.push.dispatch`, handles token resolution, and routes pushes to FCM.
9.  **backfill-jobs (Phase 1)**: Packaged as Kubernetes `CronJobs` or run-once `Jobs` rather than long-running services.

---

### 2. Infrastructure & Orchestration

#### Kubernetes Topology & Workload Classification
Deploy the services inside a Kubernetes cluster with dedicated node pools:
*   **Stateless Node Pool (General Purpose)**: Runs the API Gateway, auth, users, posts, feed, and DM services. Optimized for fast startup times.
*   **Worker Node Pool (Compute Optimized)**: Dedicated to running the Kafka consumers (search indexers and notification workers) to prevent background work from competing with user-facing request threads.
*   **Stateful Node Pool (Storage Optimized)**: Runs stateful-adjacent workloads (e.g., Redis replicas or local dev databases) on NVMe-backed instances.

#### Data Store Strategy: Managed vs Self-Hosted
*   **Cassandra**: **Managed Service** (e.g., DataStax Astra DB or AWS Keyspaces) is recommended. Running a production Cassandra cluster in-house requires significant operational overhead (compaction tuning, backup management, repairs).
*   **Kafka**: **Managed Service** (e.g., Confluent Cloud or AWS MSK) is recommended. Managed brokers handle partition rebalancing, software patches, and storage scaling seamlessly.
*   **Redis**: **Managed Service** (e.g., AWS ElastiCache or Redis Labs) configured as a clustered Redis deployment with multi-AZ failover.
*   **Elasticsearch**: **Managed Service** (e.g., Elastic Cloud) for ease of index scaling and built-in monitoring tools.

#### Service Mesh Decision
*   **Decision**: Deploy **Istio** in ambient (sidecar-less) mode.
*   **Justification**: Sidecar-less ambient mode provides transparent mTLS and network policy enforcement without the CPU/Memory overhead of injecting Envoy sidecars into every pod. This is crucial for keeping SSE connections lightweight.

---

### 3. CI/CD Pipeline

#### Per-Service Build & Scanning
Every microservice has a dedicated Github Actions pipeline:
1.  **Build**: Compiles the Go binary and builds a minimal Docker image (using a `distroless` base image to reduce attack surface).
2.  **Scan**: Runs `Trivy` for image vulnerability scanning and `govulncheck` for code dependencies.
3.  **SBOM**: Generates a Software Bill of Materials (SBOM) using `syft`.
4.  **Sign**: Signs the container image with `Cosign` to verify image authenticity before deployment.

#### Progressive Delivery & Schema Gating
*   **Canary Deployment**: Use **Argo Rollouts** to route a small percentage of traffic (e.g., 5%) to new versions, monitoring HTTP error rates and SLO burn rates.
*   **Schema Migration Gating**: Schema migrations must be additive. The database migration job (`cassandra-db-init`) runs as a pre-deploy hook. Migrations must never drop tables or rename columns, ensuring that older versions of the service continue to run during a rollback.

---

### 4. Secrets & Config Management

*   **Secrets Provider**: Deploy the **External Secrets Operator (ESO)** in Kubernetes.
*   **Flow**: ESO syncs credentials from AWS Secrets Manager or Vault into Kubernetes `Secret` resources, which are then mounted as files or injected as environment variables into pods.
*   **Key Rotation**: Enable automated key rotation in Secrets Manager. The application must refresh its cache of DB/Kafka credentials periodically.

---

### 5. Observability Stack

*   **Metrics**: Prometheus agents scrape `/metrics` from all pods, writing to a centralized **Grafana Mimir** database.
*   **Tracing**: OpenTelemetry SDKs are integrated into Go API servers, Kafka consumers, and DB queries. Tracing spans are sent to **Tempo**.
*   **Logging**: App logs are outputted to `stdout` in JSON format, shipped via FluentBit to **Grafana Loki**.

#### SLO Definitions
1.  **Feed Latency**: 95% of feed requests must complete in `< 250ms`.
2.  **Indexer Lag**: The difference between the latest Kafka offset and the committed indexer offset must be `< 500` messages.
3.  **SSE Connection Stability**: Reconnection rate must be `< 1%` per minute.
4.  **Notification Delivery Latency**: 99% of push notifications must be delivered to FCM within `< 3 seconds` of the triggering event.

---

### 6. Scaling & Resilience Plan

*   **Horizontal Pod Autoscaler (HPA)**:
    *   *Web Services*: Scale based on Average CPU Utilization (> 70%) and HTTP Request Latency.
    *   *Workers*: Scale based on Kafka consumer lag metrics (e.g., if lag on `notification.events` exceeds 5000 messages, scale up the `notif-dispatcher-worker` pods).
*   **Circuit Breakers**: Implement circuit breakers using libraries like `sony/gobreaker` around the Nominatim geocoding client and FCM push service. If Nominatim times out repeatedly, the circuit opens, bypassing geocoding for 30 seconds to prevent thread starvation.

---

## ROLLOUT ROADMAP

| Phase | Objective | Entry Criteria | Exit Criteria | Key Deliverables |
| :--- | :--- | :--- | :--- | :--- |
| **Phase 0** | **Harden & Observe**<br>(Monolith Hardening) | Assessment approved. | 100% secrets migrated to Vault/ESO. Structured JSON logging active. Prometheus metrics enabled. | Kubernetes secrets integration, OpenTelemetry tracing, standard Prometheus exporters. |
| **Phase 1** | **Worker Decoupling**<br>(Decouple Consumers) | Phase 0 exit criteria met. | Consumers running in independent containers. Monolith no longer running background consumer loops. | Dedicated `search-indexer` and `notifications-service` containers, Kafka lag monitoring dashboards. |
| **Phase 2** | **Gateway & Core Splits**<br>(Domain Isolation) | Phase 1 exit criteria met. | REST monolith broken into `auth`, `users`, `posts`, and `feed` services. All traffic routed via API Gateway. | Envoy API Gateway deployment, independent repos/StatefulSets for core services. |
| **Phase 3** | **Specialized Isolation**<br>(Security & Compliance) | Phase 2 exit criteria met. | `dm-service` and `moderation-service` isolated with strict network policies. | Isolated message storage schema, compliance firewalls, KMS integration. |
| **Phase 4** | **Mesh & Advanced Resilience** | Phase 3 exit criteria met. | Istio ambient mesh enforcing mTLS. Chaos testing verifies system reliability under node failures. | Ambient mesh deployment, automated chaos experiments (Chaos Mesh). |

---

## DECISIONS REQUIRING STAKEHOLDER SIGN-OFF

To proceed with this plan, the following decisions must be formally approved by project stakeholders:

1.  **Managed vs Self-Hosted Infrastructure**: Confirm the use of managed database and messaging services (Astra DB, AWS MSK, ElastiCache) versus self-hosting stateful components on Kubernetes.
2.  **Geocoding Compliance & Hosting**: Approve budget for hosting private OSM/Nominatim instances to ensure GDPR compliance, rather than using the public free API.
3.  **Service Mesh Choice**: Approve the adoption of Istio Ambient Mesh for mTLS security.
4.  **Disaster Recovery SLA**: Define and sign off on Recovery Time Objective (RTO) and Recovery Point Objective (RPO) targets for regional failover scenarios.
