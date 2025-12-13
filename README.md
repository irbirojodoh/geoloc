# geoloc - Hyper-Local Social Media Backend

A high-performance geospatial social media backend built with Go, PostGIS, and PostgreSQL.

## Features

- 🌍 **Geospatial Posts**: Store posts with precise latitude/longitude coordinates
- 📍 **Proximity-Based Feed**: Get posts sorted by physical distance using PostGIS
- ⚡ **High Performance**: Using pgx driver and GiST spatial indexes
- 🚀 **Production Ready**: Docker-based setup with PostGIS and Redis

## Tech Stack

- **Language**: Go 1.21+
- **Web Framework**: Gin
- **Database**: PostgreSQL 15 + PostGIS 3.3
- **Cache**: Redis (for future use)
- **Driver**: pgx/v5 (high-performance PostgreSQL driver)

## Project Structure

```
geoloc/
├── cmd/
│   └── api/
│       └── main.go              # Server entry point
├── internal/
│   ├── data/
│   │   ├── models.go            # Data models
│   │   └── query.go             # Database queries & repositories
│   └── handlers/
│       └── post.go              # HTTP handlers
├── migrations/
│   ├── 000001_init_schema.up.sql
│   └── 000001_init_schema.down.sql
├── docker-compose.yml
├── .env
└── go.mod
```

## Quick Start

### 1. Install Dependencies

```bash
go mod download
```

### 2. Start Infrastructure

```bash
docker compose up -d
```

This starts:
- PostgreSQL with PostGIS on port 5432
- Redis on port 6379

### 3. Run Migrations

The migrations will be automatically applied when PostgreSQL starts (via docker-entrypoint-initdb.d).

Alternatively, you can manually apply them:

```bash
docker exec -i geoloc_postgres psql -U user -d geobackend < migrations/000001_init_schema.up.sql
```

### 4. Start the Server

```bash
go run cmd/api/main.go
```

The API will be available at `http://localhost:8080`

## API Endpoints

### Health Check

Check if the API is running.

**Endpoint:** `GET /health`

**Response:**
```json
{
  "status": "ok"
}
```

**Example:**
```bash
curl http://localhost:8080/health
```

---

### User Endpoints

#### 1. Create User

Create a new user account.

**Endpoint:** `POST /api/v1/users`

**Request Headers:**
- `Content-Type: application/json`

**Request Body:**
---

## Complete Usage Example

### Step 1: Create Users

```bash
# Create first user
curl -X POST http://localhost:8080/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{
    "username": "alice",
    "email": "alice@example.com",
    "full_name": "Alice Smith",
    "bio": "NYC Explorer"
  }'

# Create second user
curl -X POST http://localhost:8080/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{
    "username": "bob",
    "email": "bob@example.com",
    "full_name": "Bob Johnson",
    "bio": "LA Travel Blogger"
  }'
```

### Step 2: Create Posts in Different Locations

```bash
# Post from Central Park, NYC
curl -X POST http://localhost:8080/api/v1/posts \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": 1,
    "content": "Beautiful morning in Central Park! 🌳",
    "latitude": 40.785091,
    "longitude": -73.968285
  }'

# Post from Times Square, NYC
curl -X POST http://localhost:8080/api/v1/posts \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": 1,
    "content": "The lights of Times Square never get old ✨",
    "latitude": 40.758896,
    "longitude": -73.985130
  }'

# Post from Santa Monica Pier, LA
curl -X POST http://localhost:8080/api/v1/posts \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": 2,
    "content": "Sunset at Santa Monica Pier 🌅",
    "latitude": 34.0095,
    "longitude": -118.4988
  }'
```

### Step 3: Get Nearby Feed

```bash
# Get posts near Central Park (within 10km)
curl "http://localhost:8080/api/v1/feed?latitude=40.785091&longitude=-73.968285&radius_km=10&limit=20"

# Get posts near Santa Monica (within 5km)
curl "http://localhost:8080/api/v1/feed?latitude=34.0095&longitude=-118.4988&radius_km=5"
```

### Step 4: Lookup User Details

```bash
# Get user by ID
curl http://localhost:8080/api/v1/users/1

# Get user by username
curl http://localhost:8080/api/v1/users/username/alice
**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{
    "username": "johndoe",
    "email": "john@example.com",
    "full_name": "John Doe",
    "bio": "Tech enthusiast from NYC"
  }'
```

#### 2. Get User by ID

Retrieve a user by their ID.

**Endpoint:** `GET /api/v1/users/:id`

**URL Parameters:**
- `id` (integer): User ID

**Success Response (200 OK):**
```json
{
  "user": {
    "id": 1,
    "username": "johndoe",
    "email": "john@example.com",
    "full_name": "John Doe",
    "bio": "Tech enthusiast",
    "created_at": "2025-12-13T19:30:00Z",
    "updated_at": "2025-12-13T19:30:00Z"
  }
}
```

**Error Responses:**
- `400 Bad Request`: Invalid user ID format
- `404 Not Found`: User not found
- `500 Internal Server Error`: Database error

**Example:**
```bash
curl http://localhost:8080/api/v1/users/1
```

#### 3. Get User by Username

Retrieve a user by their username.

**Endpoint:** `GET /api/v1/users/username/:username`

**URL Parameters:**
- `username` (string): Username

**Success Response (200 OK):**
```json
{
  "user": {
    "id": 1,
    "username": "johndoe",
    "email": "john@example.com",
    "full_name": "John Doe",
    "bio": "Tech enthusiast",
    "created_at": "2025-12-13T19:30:00Z",
    "updated_at": "2025-12-13T19:30:00Z"
  }
}
```

**Error Responses:**
- `404 Not Found`: User not found
- `500 Internal Server Error`: Database error

**Example:**
```bash
curl http://localhost:8080/api/v1/users/username/johndoe
```

---

### Post Endpoints

#### 4. Create Post

Create a new post with geolocation.

**Endpoint:** `POST /api/v1/posts`

**Request Headers:**
- `Content-Type: application/json`

**Request Body:**
```json
{
  "user_id": 1,                 // required
  "content": "Hello world!",    // required
  "latitude": 40.758896,        // required, -90 to 90
  "longitude": -73.985130       // required, -180 to 180
}
```

**Success Response (201 Created):**
```json
{
  "message": "Post created successfully",
  "post": {
    "id": 1,
    "user_id": 1,
    "content": "Hello from Times Square!",
    "latitude": 40.758896,
    "longitude": -73.985130,
    "created_at": "2025-12-13T19:30:00Z"
  }
}
```

**Error Responses:**
- `400 Bad Request`: Invalid request body, missing fields, or invalid coordinates
- `500 Internal Server Error`: Database error

**Coordinate Validation:**
- Latitude must be between -90 and 90
- Longitude must be between -180 and 180

**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/posts \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": 1,
    "content": "Amazing view from Central Park!",
    "latitude": 40.785091,
    "longitude": -73.968285
  }'
```

#### 5. Get Nearby Feed

Retrieve posts sorted by proximity to a given location.

**Endpoint:** `GET /api/v1/feed`

**Query Parameters:**
- `latitude` (float, required): Your current latitude (-90 to 90)
- `longitude` (float, required): Your current longitude (-180 to 180)
- `radius_km` (float, optional): Search radius in kilometers (default: 10)
- `limit` (integer, optional): Max number of posts to return (default: 50, max: 100)

**Success Response (200 OK):**
```json
{
  "message": "Feed fetched successfully",
  "count": 2,
  "posts": [
    {
      "id": 1,
      "user_id": 1,
      "content": "Hello from Times Square!",
      "latitude": 40.758896,
      "longitude": -73.985130,
      "created_at": "2025-12-13T19:25:00Z"
    },
    {
      "id": 2,
      "user_id": 2,
      "content": "Amazing view from Central Park!",
      "latitude": 40.785091,
      "longitude": -73.968285,
      "created_at": "2025-12-13T19:30:00Z"
    }
  ]
}
```

**Error Responses:**
- `400 Bad Request`: Missing required parameters or invalid coordinates
- `500 Internal Server Error`: Database error

**Example:**
```bash
# Get posts within 5km radius, limit to 20 results
curl "http://localhost:8080/api/v1/feed?latitude=40.758896&longitude=-73.985130&radius_km=5&limit=20"

# Get posts within default 10km radius
curl "http://localhost:8080/api/v1/feed?latitude=40.758896&longitude=-73.985130"
```

**Notes:**
- Posts are sorted by distance (nearest first)
- Uses PostGIS spatial indexes for high performance
- Distance calculations use the Haversine formula via PostGIS

## Example Usage

### Create posts in different locations

```bash
# New York
curl -X POST http://localhost:8080/api/v1/posts \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": 1,
    "content": "Amazing view from Central Park!",
    "latitude": 40.785091,
    "longitude": -73.968285
  }'

# Los Angeles
curl -X POST http://localhost:8080/api/v1/posts \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": 2,
    "content": "Sunset at Santa Monica Pier",
    "latitude": 34.0095,
    "longitude": -118.4988
  }'
```

### Get feed near Central Park

```bash
curl "http://localhost:8080/api/v1/feed?latitude=40.785091&longitude=-73.968285&radius_km=10&limit=20"
```

## How Geospatial Queries Work

The application uses PostGIS for high-performance geospatial operations:

1. **Storage**: Posts are stored with `GEOGRAPHY(POINT, 4326)` type (WGS84 coordinate system)
2. **Spatial Index**: GiST index on the location column enables fast nearest neighbor search
3. **Query Optimization**: 
   - `ST_DWithin` filters posts within radius (uses spatial index)
   - `ST_Distance` calculates exact distance for sorting
   - Results are ordered by proximity (nearest first)

## Environment Variables

Create a `.env` file or set these environment variables:

```env
DB_HOST=localhost
DB_PORT=5432
DB_USER=user
DB_PASSWORD=password
DB_NAME=geobackend
PORT=8080
```

## Development

### Run with hot reload

```bash
# Install air for hot reload
go install github.com/cosmtrek/air@latest

# Run with air
air
```

### Run tests

```bash
go test ./...
```

## Production Considerations

- [ ] Add authentication/authorization
- [ ] Implement rate limiting
- [ ] Add Redis caching for hot feeds
- [ ] Set up proper logging (structured logs)
- [ ] Add monitoring and metrics
- [ ] Implement database connection pooling tuning
- [ ] Add comprehensive error handling
- [ ] Set up CI/CD pipeline

## License

MIT
