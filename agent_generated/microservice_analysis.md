# Geoloc — Monolith to Microservices Transition Analysis

## 1. Current Architectural State
Geoloc is currently operating as a **Modular Monolith**. 
- **Compute**: A single Go application (compiled to one binary) handling all HTTP routes.
- **Data**: A single Cassandra Keyspace (`geoloc`) and a shared Redis cluster.
- **Routing**: Caddy acting as a basic reverse proxy and TLS terminator.

**Why this is good for MVP**: The modular monolith pattern (separated by packages like `auth`, `data`, `handlers`) allows for maximum developer velocity, easy deployment (one container), and lack of network latency between internal domains.

---

## 2. When to Transition? (The Triggers)
Moving to microservices introduces significant operational complexity (distributed tracing, CI/CD per service, network failures, eventual consistency). You should **only** transition when you hit these triggers:
1. **Organizational Scaling**: When you have multiple engineering teams (e.g., > 10 backend engineers) stepping on each other's toes in the same repository.
2. **Independent Scaling**: If the "Feed Retrieval" logic requires 50x more compute resources than "User Registration", but you currently have to scale the entire monolith to handle the feed traffic.
3. **Different Tech Stacks**: If you need to build an AI/ML feed recommendation engine, you might want to write it in Python, while keeping the core API in Go.
4. **Deployment Friction**: If testing and deploying the entire monolith takes too long or introduces too much risk for a small feature change.

---

## 3. Proposed Microservice Boundaries (Domain-Driven Design)

If/when the transition is required, the application should be split along its natural domain boundaries:

### 🛡️ Identity & Access Management (IAM) Service
- **Responsibilities**: User registration, login, JWT issuance, OAuth, password reset, profile management, and account deletion.
- **Data**: `users`, `password_reset_tokens`
- **Dependencies**: Redis (brute force limits).

### 🌍 Geospatial Core Service (Feed & Posts)
- **Responsibilities**: Post creation, feed generation via geohash proximity, Nominatim geocoding integration.
- **Data**: `posts_by_id`, `posts_by_user`, `posts_by_geohash`
- **Dependencies**: Cassandra, IAM Service (to validate author info), Interaction Service (to attach likes/comments counts).

### 💬 Social Interaction Service
- **Responsibilities**: Likes, Comments, Follows, Location Follows.
- **Data**: `comments`, `likes`, `like_state`, `like_counts`, `follows`, `followers`, `location_follows`
- **Dependencies**: Redis (atomic counters), Cassandra.

### 🚔 Trust & Safety (Moderation) Service
- **Responsibilities**: Reports, blocks, mutes.
- **Data**: `reports`, `blocks`, `mutes`
- **Notes**: The Feed Service will need to query this service (or consume its events) to filter out blocked/muted content quickly.

### 🔔 Notification Service
- **Responsibilities**: In-app notifications list, Apple APNs / Firebase FCM push notification dispatch.
- **Data**: `notifications`
- **Notes**: Highly asynchronous. Should consume events via a message broker rather than being called via HTTP.

### 📁 Media Service
- **Responsibilities**: Image/Video upload, resizing, compression, S3 integration, CDN invalidation.
- **Data**: S3 / Object Storage (Metadata stored in Cassandra).

---

## 4. Architectural Shifts Required

### A. API Gateway Pattern
Caddy is great for a monolith, but microservices require a dedicated **API Gateway** (e.g., KrakenD, Kong, or Envoy).
- **Role**: Route `/api/v1/users` to IAM, `/api/v1/feed` to Feed Service.
- **Auth Offloading**: The Gateway should validate the JWT and forward the user's ID to downstream services, removing the need for every microservice to decode JWTs.

### B. Inter-Service Communication
- **Synchronous**: Use **gRPC**. For example, if the Feed Service needs to ask the Moderation Service "Are user A and B blocked?", it should use a fast gRPC call.
- **Asynchronous (Event-Driven)**: Use **Kafka or Pulsar**. When a user likes a post:
  1. Interaction Service saves the like.
  2. Interaction Service publishes an `UserLikedPost` event to Kafka.
  3. Notification Service consumes the event and sends a push notification.

### C. Database per Service & Data Duplication
Microservices should **not** share the same database tables. 
- The IAM service owns the `users` table. 
- In your current Cassandra schema, the `posts` tables duplicate user data (`username`, `profile_picture_url`) to avoid joins. 
- **Microservices Solution**: When a user updates their profile in the IAM service, it publishes a `UserProfileUpdated` event to Kafka. The Feed Service consumes this event and runs an asynchronous background job to update the duplicated username in all its `posts_by_geohash` tables.

---

## 5. Migration Strategy: The Strangler Fig Pattern

Do not attempt a "Big Bang" rewrite. Use the Strangler Fig pattern:

1. **Implement an API Gateway**: Put KrakenD or Kong in front of your current Go Monolith. Route 100% of traffic to the monolith.
2. **Extract the Easiest Service**: Extract the **Media/Upload Service** first. It has very few dependencies. Point the API gateway's `/api/v1/upload` route to the new microservice. The monolith no longer handles uploads.
3. **Extract Asynchronous Work**: Extract the **Notification Service**. Have the monolith publish events to Kafka, and let the new Go Notification microservice handle push deliveries.
4. **Extract Core Domains**: Gradually extract the IAM service, then the Interaction service, and finally the Feed service, updating the API gateway routing one by one until the monolith is entirely "strangled" and can be deleted.

---

## 6. Infrastructure Overhead

Transitioning will require significant DevOps upgrades:
- **Kubernetes (K8s)** to manage the lifecycle, scaling, and health of 6+ different services.
- **Kafka/RabbitMQ** for event-driven architecture.
- **Distributed Tracing** (Jaeger/OpenTelemetry) because debugging a 500 error that hopped across 3 different microservices is impossible with standard logs.
- **Service Mesh** (Istio/Linkerd) for internal mTLS, retries, and circuit breaking between services.
