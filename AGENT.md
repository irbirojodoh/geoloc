# AGENT.md - Geoloc Project Context

## Overview

**Geoloc** is a hyper-local social media backend built with Go and Cassandra. Features geospatial posts, social interactions, and location-based subscriptions.

---

## Tech Stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.21+ |
| Framework | Gin |
| Database | Apache Cassandra 4.1 |
| Auth | JWT (15min access, 7d refresh) |
| Password | SHA3-512 → SHA-256 |
| Geospatial | Geohashing |
| CORS | All origins allowed (`*`) |

---

## API Endpoints

### Auth (Public)
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/auth/register` | Register |
| POST | `/auth/login` | Login |
| POST | `/auth/refresh` | Refresh token |
| GET | `/api/v1/feed` | Public feed (paginated) |

### Protected (Require Bearer token)

**Profile & Users**
- `PUT /users/me` - Update profile
- `GET /users/:id` - Get user
- `GET /users/:id/posts` - User's posts

**Follows**
- `POST /users/:id/follow` - Follow
- `DELETE /users/:id/follow` - Unfollow
- `GET /users/:id/followers` - Followers
- `GET /users/:id/following` - Following

**Posts**
- `POST /posts` - Create post
- `GET /posts/:id` - Get post
- `POST/DELETE /posts/:id/like` - Like/unlike
- `POST /posts/:id/comments` - Comment
- `GET /posts/:id/comments` - Get comments

**Comments**
- `POST /comments/:id/reply` - Reply (max 3 deep)
- `POST/DELETE /comments/:id/like` - Like/unlike
- `DELETE /comments/:id` - Delete

**Locations**
- `POST /locations/follow` - Follow area
- `DELETE /locations/:geohash/follow` - Unfollow
- `GET /locations/following` - Get followed

**Notifications**
- `GET /notifications` - Get all
- `PUT /notifications/:id/read` - Mark read
- `PUT /notifications/read-all` - Mark all read

**Search**
- `GET /search/users?q=` - Search users
- `GET /search/posts?q=` - Search posts

**Upload**
- `POST /upload/avatar` - Avatar (max 5MB)
- `POST /upload/post` - Media (max 50MB)

**Devices (Push)**
- `POST /devices` - Register token
- `DELETE /devices` - Unregister

---

## Pagination

Feed endpoint uses **cursor-based pagination**:

```
GET /api/v1/feed?latitude=-6.36&longitude=106.82&limit=20&cursor=<next_cursor>
```

**Response:**
```json
{
  "data": [/* posts with username */],
  "count": 20,
  "has_more": true,
  "next_cursor": "MjAyNi0wMS0wNVQxMDozMDowMFo="
}
```

- Posts include `username` and `profile_picture_url`
- Pass `next_cursor` to get next page
- Stop when `has_more` is `false`

---

## Features

- ✅ JWT Authentication
- ✅ Geolocation posts & feed
- ✅ Cursor-based pagination
- ✅ Likes & nested comments (3 levels)
- ✅ Follow users & locations
- ✅ Notifications
- ✅ Search (users/posts)
- ✅ Image/video upload (local, S3 ready)
- ✅ Rate limiting (100 req/min)
- ✅ Push notifications (template)

---

## Quick Start

```bash
docker compose up -d
docker cp migrations/cassandra_schema.cql geoloc_cassandra:/tmp/
docker exec -it geoloc_cassandra cqlsh -f /tmp/cassandra_schema.cql
go run cmd/api/main.go
```

**Seed test data (20 users, 200 posts):**
```bash
go run cmd/seed/main.go
```

---

## Project Structure

```
internal/data/
├── models.go              # Data models
├── pagination.go          # Cursor-based pagination
├── geohash.go             # Geospatial utilities
├── post_repo.go           # Post repository
├── user_repo.go           # User repository
├── comment_repo.go        # Comment repository
├── like_repo.go           # Like repository
├── follow_repo.go         # Follow repository
├── location_follow_query.go
└── notification_query.go
```

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CASSANDRA_HOST` | `localhost` | DB host |
| `CASSANDRA_KEYSPACE` | `geoloc` | Keyspace |
| `JWT_SECRET` | (default) | JWT secret |
| `PORT` | `8080` | Server port |
| `UPLOAD_PATH` | `./uploads` | Upload dir |
| `BASE_URL` | `http://localhost:8080` | Base URL |
