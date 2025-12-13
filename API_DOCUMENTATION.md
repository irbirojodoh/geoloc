# API Documentation

Base URL: `http://localhost:8080`

## Table of Contents

- [Health Check](#health-check)
- [User Endpoints](#user-endpoints)
  - [Create User](#1-create-user)
  - [Get User by ID](#2-get-user-by-id)
  - [Get User by Username](#3-get-user-by-username)
- [Post Endpoints](#post-endpoints)
  - [Create Post](#4-create-post)
  - [Get Nearby Feed](#5-get-nearby-feed)
- [Error Responses](#error-responses)
- [Rate Limits](#rate-limits)

---

## Health Check

Check if the API is running and healthy.

**Endpoint:** `GET /health`

**Response:** `200 OK`
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

## User Endpoints

### 1. Create User

Create a new user account.

**Endpoint:** `POST /api/v1/users`

**Headers:**
| Header | Value | Required |
|--------|-------|----------|
| Content-Type | application/json | Yes |

**Request Body:**
| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| username | string | Yes | 3-50 characters, unique |
| email | string | Yes | Valid email format, unique |
| full_name | string | No | Max 100 characters |
| bio | string | No | No limit |

**Request Example:**
```json
{
  "username": "johndoe",
  "email": "john@example.com",
  "full_name": "John Doe",
  "bio": "Tech enthusiast from NYC"
}
```

**Success Response:** `201 Created`
```json
{
  "message": "User created successfully",
  "user": {
    "id": 1,
    "username": "johndoe",
    "email": "john@example.com",
    "full_name": "John Doe",
    "bio": "Tech enthusiast from NYC",
    "created_at": "2025-12-13T19:30:00Z",
    "updated_at": "2025-12-13T19:30:00Z"
  }
}
```

**Error Responses:**

| Status Code | Description |
|-------------|-------------|
| 400 Bad Request | Invalid request body or validation failed |
| 409 Conflict | Username or email already exists |
| 500 Internal Server Error | Database error |

**curl Example:**
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

---

### 2. Get User by ID

Retrieve a user's information by their ID.

**Endpoint:** `GET /api/v1/users/:id`

**URL Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| id | integer | Yes | User ID |

**Success Response:** `200 OK`
```json
{
  "user": {
    "id": 1,
    "username": "johndoe",
    "email": "john@example.com",
    "full_name": "John Doe",
    "bio": "Tech enthusiast from NYC",
    "created_at": "2025-12-13T19:30:00Z",
    "updated_at": "2025-12-13T19:30:00Z"
  }
}
```

**Error Responses:**

| Status Code | Description |
|-------------|-------------|
| 400 Bad Request | Invalid user ID format |
| 404 Not Found | User not found |
| 500 Internal Server Error | Database error |

**curl Example:**
```bash
curl http://localhost:8080/api/v1/users/1
```

---

### 3. Get User by Username

Retrieve a user's information by their username.

**Endpoint:** `GET /api/v1/users/username/:username`

**URL Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| username | string | Yes | Username |

**Success Response:** `200 OK`
```json
{
  "user": {
    "id": 1,
    "username": "johndoe",
    "email": "john@example.com",
    "full_name": "John Doe",
    "bio": "Tech enthusiast from NYC",
    "created_at": "2025-12-13T19:30:00Z",
    "updated_at": "2025-12-13T19:30:00Z"
  }
}
```

**Error Responses:**

| Status Code | Description |
|-------------|-------------|
| 404 Not Found | User not found |
| 500 Internal Server Error | Database error |

**curl Example:**
```bash
curl http://localhost:8080/api/v1/users/username/johndoe
```

---

## Post Endpoints

### 4. Create Post

Create a new post with geolocation data.

**Endpoint:** `POST /api/v1/posts`

**Headers:**
| Header | Value | Required |
|--------|-------|----------|
| Content-Type | application/json | Yes |

**Request Body:**
| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| user_id | integer | Yes | Must be a valid user ID |
| content | string | Yes | No limit |
| latitude | float | Yes | -90 to 90 |
| longitude | float | Yes | -180 to 180 |

**Request Example:**
```json
{
  "user_id": 1,
  "content": "Amazing view from Central Park!",
  "latitude": 40.785091,
  "longitude": -73.968285
}
```

**Success Response:** `201 Created`
```json
{
  "message": "Post created successfully",
  "post": {
    "id": 1,
    "user_id": 1,
    "content": "Amazing view from Central Park!",
    "latitude": 40.785091,
    "longitude": -73.968285,
    "created_at": "2025-12-13T19:30:00Z"
  }
}
```

**Error Responses:**

| Status Code | Description |
|-------------|-------------|
| 400 Bad Request | Invalid request body, missing fields, or invalid coordinates |
| 500 Internal Server Error | Database error |

**Coordinate Validation:**
- Latitude must be between -90 (South Pole) and 90 (North Pole)
- Longitude must be between -180 and 180

**curl Example:**
```bash
curl -X POST http://localhost:8080/api/v1/posts \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": 1,
    "content": "Beautiful morning in Central Park! 🌳",
    "latitude": 40.785091,
    "longitude": -73.968285
  }'
```

---

### 5. Get Nearby Feed

Retrieve posts sorted by proximity to a given location using PostGIS spatial queries.

**Endpoint:** `GET /api/v1/feed`

**Query Parameters:**
| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| latitude | float | Yes | - | Your current latitude (-90 to 90) |
| longitude | float | Yes | - | Your current longitude (-180 to 180) |
| radius_km | float | No | 10 | Search radius in kilometers |
| limit | integer | No | 50 | Max posts to return (max: 100) |

**Success Response:** `200 OK`
```json
{
  "message": "Feed fetched successfully",
  "count": 2,
  "posts": [
    {
      "id": 2,
      "user_id": 1,
      "content": "The lights of Times Square never get old ✨",
      "latitude": 40.758896,
      "longitude": -73.985130,
      "created_at": "2025-12-13T19:28:00Z"
    },
    {
      "id": 1,
      "user_id": 1,
      "content": "Beautiful morning in Central Park! 🌳",
      "latitude": 40.785091,
      "longitude": -73.968285,
      "created_at": "2025-12-13T19:25:00Z"
    }
  ]
}
```

**Error Responses:**

| Status Code | Description |
|-------------|-------------|
| 400 Bad Request | Missing required parameters or invalid coordinates |
| 500 Internal Server Error | Database error |

**Notes:**
- Posts are ordered by distance (nearest first)
- Uses PostGIS `ST_DWithin` for efficient spatial filtering
- Uses PostGIS `ST_Distance` for distance calculation
- Leverages GiST spatial indexes for performance

**curl Examples:**
```bash
# Get posts within 5km radius, limit to 20 results
curl "http://localhost:8080/api/v1/feed?latitude=40.758896&longitude=-73.985130&radius_km=5&limit=20"

# Get posts within default 10km radius
curl "http://localhost:8080/api/v1/feed?latitude=40.758896&longitude=-73.985130"

# Get posts near Central Park
curl "http://localhost:8080/api/v1/feed?latitude=40.785091&longitude=-73.968285&radius_km=10"
```

---

## Error Responses

All error responses follow this format:

```json
{
  "error": "Error message",
  "details": "Detailed error information (optional)"
}
```

### Common HTTP Status Codes

| Status Code | Description |
|-------------|-------------|
| 200 OK | Request succeeded |
| 201 Created | Resource created successfully |
| 400 Bad Request | Invalid request (validation failed, missing parameters) |
| 404 Not Found | Resource not found |
| 409 Conflict | Conflict with existing resource (duplicate username/email) |
| 500 Internal Server Error | Server-side error |

### Example Error Responses

**400 Bad Request - Validation Error:**
```json
{
  "error": "Invalid request body",
  "details": "Key: 'CreateUserRequest.Email' Error:Field validation for 'Email' failed on the 'email' tag"
}
```

**404 Not Found:**
```json
{
  "error": "User not found"
}
```

**409 Conflict:**
```json
{
  "error": "Username or email already exists"
}
```

---

## Rate Limits

Currently, there are no rate limits implemented. This is a PoC version.

**Production Recommendations:**
- Implement rate limiting per IP address
- Use Redis for distributed rate limiting
- Suggested limits:
  - 100 requests per minute for reads
  - 30 requests per minute for writes

---

## Authentication

Currently, there is no authentication implemented. All endpoints are public.

**Production Recommendations:**
- Add JWT-based authentication
- Implement user sessions
- Add API key support for third-party integrations
- Use middleware for protected routes

---

## CORS Configuration

The API currently allows all origins (`*`). This should be restricted in production.

**Current Configuration:**
- Allow Origins: `*`
- Allow Methods: `GET`, `POST`, `PUT`, `DELETE`
- Allow Headers: `Origin`, `Content-Type`, `Authorization`

---

## Database Schema

### Users Table

| Column | Type | Constraints |
|--------|------|-------------|
| id | SERIAL | PRIMARY KEY |
| username | VARCHAR(50) | UNIQUE, NOT NULL |
| email | VARCHAR(255) | UNIQUE, NOT NULL |
| full_name | VARCHAR(100) | NULLABLE |
| bio | TEXT | NULLABLE |
| created_at | TIMESTAMP WITH TIME ZONE | DEFAULT NOW() |
| updated_at | TIMESTAMP WITH TIME ZONE | DEFAULT NOW() |

**Indexes:**
- `users_pkey`: Primary key on `id`
- `idx_users_username`: B-tree index on `username`
- `idx_users_email`: B-tree index on `email`
- `users_username_key`: Unique constraint on `username`
- `users_email_key`: Unique constraint on `email`

### Posts Table

| Column | Type | Constraints |
|--------|------|-------------|
| id | SERIAL | PRIMARY KEY |
| user_id | INTEGER | FOREIGN KEY (users.id) ON DELETE CASCADE |
| content | TEXT | NOT NULL |
| location | GEOGRAPHY(POINT, 4326) | NOT NULL |
| created_at | TIMESTAMP WITH TIME ZONE | DEFAULT NOW() |

**Indexes:**
- `posts_pkey`: Primary key on `id`
- `idx_posts_location`: GiST spatial index on `location`
- `idx_posts_created_at`: B-tree index on `created_at DESC`
- `idx_posts_user_id`: B-tree index on `user_id`

**Foreign Keys:**
- `fk_posts_user_id`: References `users(id)` with CASCADE delete

---

## Testing with Postman

Import the following collection to test all endpoints:

### Postman Collection (JSON)

```json
{
  "info": {
    "name": "Geoloc API",
    "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"
  },
  "item": [
    {
      "name": "Health Check",
      "request": {
        "method": "GET",
        "header": [],
        "url": {
          "raw": "http://localhost:8080/health",
          "protocol": "http",
          "host": ["localhost"],
          "port": "8080",
          "path": ["health"]
        }
      }
    },
    {
      "name": "Create User",
      "request": {
        "method": "POST",
        "header": [
          {
            "key": "Content-Type",
            "value": "application/json"
          }
        ],
        "body": {
          "mode": "raw",
          "raw": "{\n  \"username\": \"johndoe\",\n  \"email\": \"john@example.com\",\n  \"full_name\": \"John Doe\",\n  \"bio\": \"Tech enthusiast\"\n}"
        },
        "url": {
          "raw": "http://localhost:8080/api/v1/users",
          "protocol": "http",
          "host": ["localhost"],
          "port": "8080",
          "path": ["api", "v1", "users"]
        }
      }
    },
    {
      "name": "Get User by ID",
      "request": {
        "method": "GET",
        "header": [],
        "url": {
          "raw": "http://localhost:8080/api/v1/users/1",
          "protocol": "http",
          "host": ["localhost"],
          "port": "8080",
          "path": ["api", "v1", "users", "1"]
        }
      }
    },
    {
      "name": "Create Post",
      "request": {
        "method": "POST",
        "header": [
          {
            "key": "Content-Type",
            "value": "application/json"
          }
        ],
        "body": {
          "mode": "raw",
          "raw": "{\n  \"user_id\": 1,\n  \"content\": \"Hello from Central Park!\",\n  \"latitude\": 40.785091,\n  \"longitude\": -73.968285\n}"
        },
        "url": {
          "raw": "http://localhost:8080/api/v1/posts",
          "protocol": "http",
          "host": ["localhost"],
          "port": "8080",
          "path": ["api", "v1", "posts"]
        }
      }
    },
    {
      "name": "Get Feed",
      "request": {
        "method": "GET",
        "header": [],
        "url": {
          "raw": "http://localhost:8080/api/v1/feed?latitude=40.785091&longitude=-73.968285&radius_km=10&limit=20",
          "protocol": "http",
          "host": ["localhost"],
          "port": "8080",
          "path": ["api", "v1", "feed"],
          "query": [
            {
              "key": "latitude",
              "value": "40.785091"
            },
            {
              "key": "longitude",
              "value": "-73.968285"
            },
            {
              "key": "radius_km",
              "value": "10"
            },
            {
              "key": "limit",
              "value": "20"
            }
          ]
        }
      }
    }
  ]
}
```

Save this as `geoloc-api.postman_collection.json` and import it into Postman.

---

## Support

For issues or questions, please open an issue on GitHub.

## License

MIT
