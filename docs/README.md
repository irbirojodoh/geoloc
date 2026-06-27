# Geoloc Documentation

Welcome to the Geoloc documentation. This folder contains comprehensive guides for developers working with the Geoloc API and building client applications.

## Table of Contents

### Getting Started
- [Quick Start Guide](./getting-started.md) - Set up and run the project
- [Environment Configuration](./environment.md) - Environment variables reference

### API Reference
- [API Overview](./api/README.md) - API conventions and authentication
- [Authentication](./api/authentication.md) - Login, register, JWT tokens
- [Feed](./api/feed.md) - Location-based feed endpoints
- [Posts](./api/posts.md) - Create, read, like posts
- [Users](./api/users.md) - User profiles and follows
- [Comments](./api/comments.md) - Nested comments system
- [Notifications](./api/notifications.md) - In-app notifications and SSE
- [Direct messages (E2EE)](./api/dm.md) - Encrypted DMs and real-time delivery
- [Push notification testing](./testing-push-notifications.md) - FCM / Postman local testing
- [Search](./api/search.md) - Elasticsearch search, indexing pipeline, and backfill
- [Media & Upload](./api/media.md) - R2 uploads and presigned direct-to-R2 serving

### Architecture
- [System Overview](./architecture/overview.md) - High-level architecture
- [System Design](./architecture/system_design.md) - Data flows and component diagram
- [Database Schema](./architecture/database.md) - Cassandra tables and design
- [Direct messages (E2EE)](./architecture/dm.md) - Encrypted DM architecture
- [Geohashing](./architecture/geohashing.md) - Location-based queries

### Deployment
- [Docker Deployment](./deployment/docker.md) - Run with Docker Compose
- [Production Guide](./deployment/production.md) - Production considerations

### Client Development
- [Flutter Client Guide](./client/flutter.md) - Build the Flutter mobile app
- [API Integration](./client/api-integration.md) - Best practices for clients
- [Media (frontend)](./client/media-frontend.md) - Upload, attach keys, and display presigned URLs
- [Notifications list (frontend Phase 1)](./client/notifications-list-frontend.md) - `GET /api/v1/notifications` implementation guide

---

## Quick Links

| Resource | Description |
|----------|-------------|
| [API_DOCUMENTATION.md](../API_DOCUMENTATION.md) | Full API reference |
| [Postman Collection](../postman_collection.json) | Import into Postman |
| [Docker Compose](../docker-compose.yml) | Local development setup |
