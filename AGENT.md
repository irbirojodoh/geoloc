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
├── cmd/
│   └── api/
│       └── main.go              # Server entry point
├── internal/
│   ├── auth/
│   │   ├── password.go          # SHA3-512 → SHA-256 hashing
│   │   ├── jwt.go               # Token generation/validation
│   │   └── middleware.go        # Auth middleware
│   ├── data/
│   │   ├── models.go            # Data models (User, Post)
│   │   ├── geohash.go           # Geohash utilities
│   │   ├── query.go             # PostRepository
│   │   └── user_query.go        # UserRepository
│   └── handlers/
│       ├── auth.go              # Register, Login, Refresh
│       ├── post.go              # Post handlers
│       └── user.go              # User handlers
├── migrations/
│   └── cassandra_schema.cql     # Cassandra schema
├── docker-compose.yml
├── .env.example
└── go.mod
```

---

## Authentication

### Password Hashing
```
password → SHA3-512(h1) → SHA-256(h2) → stored
```

### Token System
| Token   | Expiry  | Purpose              |
|---------|---------|----------------------|
| Access  | 15 min  | API requests         |
| Refresh | 7 days  | Get new access token |

### Auth Endpoints (Public)
| Method | Endpoint         | Description                    |
|--------|------------------|--------------------------------|
| POST   | `/auth/register` | Create user with password      |
| POST   | `/auth/login`    | Login (email or username)      |
| POST   | `/auth/refresh`  | Refresh access token           |

### Register Request
```json
{
  "username": "johndoe",
  "email": "john@example.com",
  "password": "secret123",
  "full_name": "John Doe",
  "phone_number": "+6281234567890"
}
```

### Login Request
```json
{
  "identifier": "john@example.com",  // or "johndoe"
  "password": "secret123"
}
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
| Method | Endpoint                       | Description          |
|--------|--------------------------------|----------------------|
| GET    | `/api/v1/users/:id`            | Get user by ID       |
| GET    | `/api/v1/users/username/:name` | Get user by username |
| GET    | `/api/v1/users/:id/posts`      | Get user's posts     |
| POST   | `/api/v1/posts`                | Create post          |
| GET    | `/api/v1/posts/:id`            | Get post by ID       |

---

## Database Schema

### Users Table
```cql
CREATE TABLE users (
    id UUID PRIMARY KEY,
    username TEXT, email TEXT,
    full_name TEXT, bio TEXT,
    phone_number TEXT,
    profile_picture_url TEXT,
    password_hash TEXT,
    created_at TIMESTAMP, updated_at TIMESTAMP
);
```

### Posts Tables
- `posts_by_geohash` - Proximity queries (partition: geohash_prefix)
- `posts_by_id` - Direct lookups (partition: post_id)
- `posts_by_user` - User profile (partition: user_id)

---

## Environment Variables

| Variable             | Default     | Description          |
|----------------------|-------------|----------------------|
| `CASSANDRA_HOST`     | `localhost` | Cassandra host       |
| `CASSANDRA_KEYSPACE` | `geoloc`    | Keyspace name        |
| `JWT_SECRET`         | (default)   | JWT signing secret   |
| `PORT`               | `8080`      | API server port      |

---

## Quick Commands

```bash
# Start Cassandra
docker compose up -d

# Apply schema (wait ~60s for Cassandra to start)
docker cp migrations/cassandra_schema.cql geoloc_cassandra:/tmp/
docker exec -it geoloc_cassandra cqlsh -f /tmp/cassandra_schema.cql

# Run server
go run cmd/api/main.go

# Test register
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"test","email":"test@example.com","password":"secret123","full_name":"Test User"}'

# Test login
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"identifier":"test@example.com","password":"secret123"}'
```

---

## Dependencies

| Package               | Purpose                    |
|-----------------------|----------------------------|
| `gin-gonic/gin`       | Web framework              |
| `gocql/gocql`         | Cassandra driver           |
| `golang-jwt/jwt/v5`   | JWT tokens                 |
| `golang.org/x/crypto` | SHA3-512 hashing           |
| `mmcloughlin/geohash` | Geohash encoding           |

---

## Future Enhancements

- [ ] Add Redis geo-indexing for faster queries
- [ ] Implement cursor-based pagination
- [ ] Add structured logging
- [ ] Add rate limiting
- [ ] Create user update/delete endpoints
