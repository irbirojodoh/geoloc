# AGENT.md - Geoloc Project Context

## Project Overview

**Geoloc** is a hyper-local social media backend built with Go and Cassandra. It enables geospatial-based social features where users can create posts with location data and retrieve feeds based on physical proximity using **geohashing**.

---

## Tech Stack

| Component        | Technology                          |
|------------------|-------------------------------------|
| Language         | Go 1.21+                            |
| Web Framework    | Gin                                 |
| Database         | Apache Cassandra 4.1                |
| Cache            | Redis (infrastructure ready)        |
| Authentication   | JWT (golang-jwt/jwt/v5)             |
| Password Hash    | SHA3-512 → SHA-256 (double chain)   |
| Geospatial       | Geohashing (mmcloughlin/geohash)    |

---

## Project Structure

```
geoloc/
├── cmd/api/main.go
├── internal/
│   ├── auth/
│   │   ├── password.go, jwt.go, middleware.go
│   ├── data/
│   │   ├── models.go, geohash.go
│   │   ├── query.go, user_query.go
│   │   ├── like_query.go, comment_query.go
│   │   ├── follow_query.go, location_follow_query.go
│   │   └── notification_query.go
│   └── handlers/
│       ├── auth.go, user.go, post.go
│       ├── like.go, comment.go
│       ├── profile.go, follow.go
│       ├── location.go, notification.go
│       └── search.go
├── migrations/cassandra_schema.cql
└── docker-compose.yml
```

---

## API Endpoints

### Public Routes
| Method | Endpoint         | Description          |
|--------|------------------|----------------------|
| GET    | `/health`        | Health check         |
| GET    | `/api/v1/feed`   | Get nearby posts     |
| POST   | `/auth/register` | Register user        |
| POST   | `/auth/login`    | Login                |
| POST   | `/auth/refresh`  | Refresh token        |

### Protected Routes (Require `Authorization: Bearer <token>`)

**Profile & Users**
| Method | Endpoint | Description |
|--------|----------|-------------|
| PUT | `/users/me` | Update profile |
| GET | `/users/:id` | Get user |
| GET | `/users/username/:name` | Get by username |
| GET | `/users/:id/posts` | User's posts |

**Follows**
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/users/:id/follow` | Follow user |
| DELETE | `/users/:id/follow` | Unfollow |
| GET | `/users/:id/followers` | Get followers |
| GET | `/users/:id/following` | Get following |

**Posts**
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/posts` | Create post |
| GET | `/posts/:id` | Get post |
| POST | `/posts/:id/like` | Like post |
| DELETE | `/posts/:id/like` | Unlike |
| POST | `/posts/:id/comments` | Comment |
| GET | `/posts/:id/comments` | Get comments |

**Comments**
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/comments/:id/reply` | Reply (max 3 deep) |
| POST | `/comments/:id/like` | Like comment |
| DELETE | `/comments/:id/like` | Unlike |
| DELETE | `/comments/:id` | Delete |

**Location Follows**
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/locations/follow` | Follow area |
| DELETE | `/locations/:geohash/follow` | Unfollow |
| GET | `/locations/following` | Get followed |

**Notifications**
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/notifications` | Get all |
| PUT | `/notifications/:id/read` | Mark read |
| PUT | `/notifications/read-all` | Mark all read |

**Search**
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/search/users?q=` | Search users |
| GET | `/search/posts?q=` | Search posts |

---

## Database Tables

| Table | Purpose |
|-------|---------|
| `users` | User profiles, auth |
| `posts_by_geohash` | Proximity queries |
| `posts_by_id` | Direct lookups |
| `posts_by_user` | User's posts |
| `likes` | Post/comment likes |
| `like_counts` | Like counters |
| `comments` | Nested comments |
| `comments_by_id` | Comment lookups |
| `comment_counts` | Comment counters |
| `follows` | Who user follows |
| `followers` | User's followers |
| `follow_counts` | Follow counters |
| `location_follows` | Subscribed areas |
| `notifications` | User notifications |

---

## Quick Commands

```bash
# Start infrastructure
docker compose up -d

# Apply schema
docker cp migrations/cassandra_schema.cql geoloc_cassandra:/tmp/
docker exec -it geoloc_cassandra cqlsh -f /tmp/cassandra_schema.cql

# Run server
go run cmd/api/main.go
```

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CASSANDRA_HOST` | `localhost` | Cassandra host |
| `CASSANDRA_KEYSPACE` | `geoloc` | Keyspace |
| `JWT_SECRET` | (default) | JWT secret |
| `PORT` | `8080` | Server port |
