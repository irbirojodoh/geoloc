# Geoloc System Architecture & Data Flow

This document provides a comprehensive overview of Geoloc's technology stack, system architecture, and how data flows through the system across various core operations.

## 1. Technology Stack Overview

Geoloc is designed for high scalability, real-time performance, and hyper-local data querying. 

| Component | Technology | Purpose in System |
|-----------|------------|-------------------|
| **API Backend** | **Go (Golang)** + **Gin** | Highly concurrent, stateless HTTP REST API and SSE endpoint handling. |
| **Primary Database** | **Apache Cassandra** | Distributed NoSQL datastore optimized for heavy write workloads and rapid read queries via denormalization. |
| **Message Broker** | **Apache Kafka** | Central event bus for decoupling core operations from asynchronous background jobs (e.g., push notifications, fan-outs). |
| **In-Memory Cache** | **Redis** | Ephemeral state management, atomic counters (likes/comments), rate limiting, and Pub/Sub for real-time SSE delivery. |
| **Reverse Proxy** | **Caddy** | Terminating SSL/TLS and reverse proxying API traffic to Go. |
| **Push Delivery** | **Firebase (FCM)** | Delivering background push notifications to iOS and Android devices. |
| **Geocoding** | **Nominatim** | Translating coordinates into human-readable locations (reverse geocoding). |

---

## 2. System Architecture Diagram

```mermaid
graph TD
    Client[Mobile / Web Clients] -->|HTTPS| Proxy[Caddy Reverse Proxy]
    
    subgraph Core Backend
        Proxy -->|REST / SSE| API[Go/Gin API Server]
        
        API -->|Read/Write| Cass[(Cassandra DB)]
        API -->|Cache / Atomic / PubSub| Redis[(Redis)]
        API -->|Produce Events| Kafka[(Kafka Event Bus)]
        API -->|DM Events offline| Kafka
    end
    
    subgraph Asynchronous Workers (Consumer Groups)
        Kafka -->|notification.events| Persister[Notif Persister]
        Kafka -->|notification.events| SSEFanout[SSE Fan-out]
        Kafka -->|notification.push.dispatch| PushDispatch[FCM Push Dispatch]
        Kafka -->|notification.nearby.fanout| NearbyFanout[Nearby Geospatial Fan-out]
        Kafka -->|posts.created| SearchIndexer[Search Indexer]
        Kafka -->|dm_messages| DMPushHook[DM Push Hook future]
        SearchIndexer --> ES[(Elasticsearch)]
    end
    
    Persister -->|Persist| Cass
    SSEFanout -->|Publish| Redis
    API -->|Publish dm events| Redis
    Redis -->|Pub/Sub dm:userId| API
    PushDispatch -->|Send| FCM[Firebase Cloud Messaging]
    NearbyFanout -->|Query Nearby| Cass
    NearbyFanout -->|Produce Individual Events| Kafka
    
    API -->|Reverse Geocode| Nominatim[Nominatim API]
```

---

## 3. Core Data Flows

### A. Location-Based Feed Retrieval

Retrieving the feed is the most critical and frequent operation. It must be blazing fast.

1. **Client Request**: The client requests `/api/v1/posts?lat=X&lng=Y&radius=Z`.
2. **Geohashing**: The Go API calculates the 5-character **Geohash** for the provided coordinates.
3. **Neighbor Calculation**: The backend calculates the 8 surrounding geohashes to account for edge cases where the user is near the border of a geohash boundary.
4. **Cassandra Query**: The API queries the heavily read-optimized denormalized table: `SELECT * FROM posts_by_geohash WHERE geohash_prefix IN (...)`.
5. **Distance Filtering**: In-memory, the Go API runs the Haversine formula to strictly filter out posts that exceed the exact requested `radius`.
6. **Enrichment**: The API fetches the authors' profiles from Redis cache (or falls back to Cassandra) and calculates the exact location name using cached Nominatim results.

### B. Post Creation & Nearby Fan-out

Creating a post triggers a complex set of background events to alert nearby users.

1. **Upload**: Client uploads an image via `/api/v1/upload/post` (or presigned PUT via `/api/v1/media/upload-url`) and receives an object `key` plus a short-lived presigned GET `url`.
2. **Creation**: Client sends `POST /api/v1/posts` with the image URL (or raw key) and coordinates.
3. **Database Write**: Go API extracts/normalizes the key to write to `posts_by_id`, `posts_by_user`, and `posts_by_geohash` tables in Cassandra.
4. **Event Dispatch**: The API instantly returns `201 Created` to the client. In the background, it publishes a `NearbyFanoutJob` to Kafka and a `PostCreatedEvent` to `posts.created` for search indexing (when `KAFKA_BROKERS` is set).
5. **Nearby Processing**: The `notif-nearby-fanout` Kafka consumer reads the job. It calculates the 9 adjacent geohashes and queries Cassandra (`location_follows` and active users) to find who is tracking that area.
6. **Individual Alerts**: For every matching user, the consumer produces a distinct `NotificationEvent` back into Kafka.

### C. Social Interactions (Likes & Comments)

1. **User Action**: User likes a post (`POST /posts/:id/toggle-like`).
2. **Atomic Counter**: Go API increments the like count in **Redis** (which is eventually synced to Cassandra).
3. **Notification Event**: The API publishes a `NotificationEvent` to Kafka's `notification.events` topic and returns `200 OK`.
4. **Persister Consumer**: The `notif-persister` consumer reads the event, applies moderation checks (muting/blocking), and persists the notification in the Cassandra `notifications_by_user` table.
5. **Push Trigger**: If the user has device tokens registered, the persister consumer immediately publishes a `PushDispatchJob` to the `notification.push.dispatch` queue.

### D. Real-Time In-App Delivery (SSE)

1. **Connection**: When the app is open, it connects to `GET /api/v1/notifications/stream`.
2. **Redis Subscription**: The Go API subscribes that specific request to a Redis Pub/Sub channel: `sse:user:{userID}`.
3. **Event Intercept**: Whenever a new notification flows through Kafka, the `notif-sse-fanout` consumer reads it and instantly publishes the JSON payload to Redis Pub/Sub.
4. **Delivery**: The active HTTP connection flushes the data to the client, triggering a real-time UI update (e.g., an in-app toast or badge increment).

### E. FCM Push Notifications & Retries

1. **Dispatch**: The `notif-push-dispatch` Kafka consumer reads `PushDispatchJob` messages.
2. **FCM API**: It utilizes the Firebase Admin Go SDK to deliver the payload to Apple (APNs) and Google (FCM).
3. **Failure Handling**: If the FCM API returns an error or timeout, the consumer wraps the job in a `PushRetryJob` and publishes it to `notification.push.retry`.
4. **Exponential Backoff**: The `notif-push-retry` consumer reads the retry queue. If the `RetryAfter` timestamp hasn't been reached, it delays processing. Once ready, it pushes it back to the main dispatch queue up to 3 times.

### F. Direct messages (E2EE)

Encrypted 1:1 chat reuses the notification SSE pipe and Cassandra denormalization patterns.

1. **Key registration**: Client generates X25519 keys locally. `PUT /api/v1/dm/keys` stores the public key in `user_public_keys` (versioned). Optional `PUT /api/v1/dm/keys/backup` stores a passphrase-wrapped identity blob for multi-device restore.
2. **Conversation open**: `POST /api/v1/dm/conversations` derives a deterministic `conversation_id` from the sorted participant UUID pair, inserts canonical + inbox rows (after block check).
3. **Send**: Client encrypts with ECDH + HKDF + AES-256-GCM. API validates ciphertext shape, verifies recipient `key_version`, records `sender_key_version`, writes a logged batch to `dm_messages` + inbox bump.
4. **Real-time**: Handler publishes JSON to Redis `dm:{recipientId}`. The active SSE connection (subscribed to `dm:{userId}`) delivers `dm_new_message` events. Presence key `sse:online:{userId}` gates Kafka fallback.
5. **Offline hook**: If recipient is not SSE-online (or `KAFKA_NOTIFICATIONS_ENABLED=true`), API publishes to Kafka topic `dm_messages` for a future push consumer.
6. **History**: `GET .../messages` paginates ciphertext rows; clients fetch historical public keys via `GET /dm/keys/:userId/versions` or `?key_version=N` using `sender_key_version` on each message.

Full design: [Direct messages architecture](./dm.md). API reference: [Direct messages API](../api/dm.md).

---

## 4. Scalability & Resilience

- **No Single Point of Failure**: Every component (Go API, Redis, Cassandra, Kafka) can be clustered and horizontally scaled.
- **Eventual Consistency via Kafka**: Spikes in social activity (e.g., a viral post getting thousands of likes) will not overwhelm the database. The API simply queues events to Kafka, which acts as a shock-absorber. Consumers process these at a manageable rate.
- **Denormalization**: Cassandra is heavily denormalized. We write data multiple times (e.g., a post is saved to `posts_by_user`, `posts_by_id`, and `posts_by_geohash`) to ensure that reads are always $O(1)$ and never require SQL-like `JOIN`s. 
- **Graceful Degradation**: If Firebase goes down, push notifications queue up in Kafka and retry gracefully without impacting the core API performance or SSE real-time delivery.
