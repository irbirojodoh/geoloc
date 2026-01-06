# Geoloc API Documentation

**Base URL:** `http://localhost:8080`

## Table of Contents

- [Overview](#overview)
- [Authentication](#authentication)
- [Public Endpoints](#public-endpoints)
- [Protected Endpoints](#protected-endpoints)
  - [Profile](#profile)
  - [Users](#users)
  - [Follows](#follows)
  - [Posts](#posts)
  - [Likes](#likes)
  - [Comments](#comments)
  - [Locations](#locations)
  - [Notifications](#notifications)
  - [Search](#search)
  - [Upload](#upload)
  - [Devices](#devices)
- [Error Responses](#error-responses)
- [Rate Limits](#rate-limits)

---

## Overview

Geoloc is a hyper-local social media API built with Go and Cassandra. It features:
- JWT-based authentication (15-min access token, 7-day refresh token)
- Geospatial posts using geohashing (~5km precision)
- Nested comments (up to 3 levels)
- User and location following
- Push notification support
- Rate limiting (100 req/min per IP)

---

## Authentication

All protected endpoints require a Bearer token in the Authorization header:

```
Authorization: Bearer <access_token>
```

### Token Lifecycle
| Token | Expiry |
|-------|--------|
| Access Token | 15 minutes |
| Refresh Token | 7 days |

---

## Public Endpoints

### Health Check

**Endpoint:** `GET /health`

**Response:** `200 OK`
```json
{
  "status": "ok",
  "database": "cassandra"
}
```

---

### Register

**Endpoint:** `POST /auth/register`

**Request Body:**
| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| username | string | Yes | 3-50 characters, unique |
| email | string | Yes | Valid email, unique |
| password | string | Yes | Min 6 characters |
| full_name | string | Yes | - |
| phone_number | string | No | - |

**Request Example:**
```json
{
  "username": "johndoe",
  "email": "john@example.com",
  "password": "securepassword123",
  "full_name": "John Doe",
  "phone_number": "+1234567890"
}
```

**Success Response:** `201 Created`
```json
{
  "message": "User registered successfully",
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "username": "johndoe",
    "email": "john@example.com",
    "full_name": "John Doe"
  },
  "access_token": "eyJ...",
  "refresh_token": "eyJ...",
  "expires_in": 900
}
```

---

### Login

**Endpoint:** `POST /auth/login`

**Request Body:**
| Field | Type | Description |
|-------|------|-------------|
| identifier | string | Email or username |
| password | string | User password |

**Request Example:**
```json
{
  "identifier": "johndoe",
  "password": "securepassword123"
}
```

**Success Response:** `200 OK`
```json
{
  "message": "Login successful",
  "access_token": "eyJ...",
  "refresh_token": "eyJ...",
  "expires_in": 900,
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "username": "johndoe",
    "email": "john@example.com"
  }
}
```

---

### Refresh Token

**Endpoint:** `POST /auth/refresh`

**Request Body:**
```json
{
  "refresh_token": "eyJ..."
}
```

**Success Response:** `200 OK`
```json
{
  "access_token": "eyJ...",
  "refresh_token": "eyJ...",
  "expires_in": 900
}
```

---

## Protected Endpoints

> All endpoints below require `Authorization: Bearer <token>` header.

---

### Get Feed

**Endpoint:** `GET /api/v1/feed`

> ⚠️ **This endpoint requires authentication** (JWT Bearer token)

This endpoint uses **cursor-based pagination** for efficient infinite scroll implementation.

**Query Parameters:**
| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| latitude | float | Yes | - | -90 to 90 |
| longitude | float | Yes | - | -180 to 180 |
| radius_km | float | No | 10 | Search radius in km |
| limit | int | No | 20 | Posts per page (max 100) |
| cursor | string | No | - | Pagination cursor (base64 encoded) |

---

#### Pagination Guide for Mobile Clients

**How It Works:**
1. First request: Omit `cursor` parameter to get newest posts
2. Response includes `next_cursor` if more posts exist
3. Subsequent requests: Pass `next_cursor` as `cursor` parameter
4. Continue until `has_more` is `false`

**Response Fields:**
| Field | Type | Description |
|-------|------|-------------|
| data | array | Array of post objects |
| count | int | Number of posts in this response |
| has_more | bool | `true` if more posts are available |
| next_cursor | string | Pass this as `cursor` for next page (empty if no more) |

**Post Object Fields:**
| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Post unique identifier |
| user_id | UUID | Author's user ID |
| username | string | Author's username |
| profile_picture_url | string | Author's avatar URL |
| content | string | Post text content |
| media_urls | array | Media attachments (max 4) |
| latitude | float | Post location latitude |
| longitude | float | Post location longitude |
| geohash | string | Geohash of location |
| location_name | string | Place name (e.g., "Kukusan") |
| address | object | Full address object (see below) |
| created_at | timestamp | ISO 8601 format |
| distance_km | float | Distance from query location |

**Address Object Fields:**
| Field | Type | Description |
|-------|------|-------------|
| village | string | Village/neighborhood name |
| city_district | string | District within city |
| city | string | City name |
| state | string | State/province |
| region | string | Region |
| postcode | string | Postal code |
| country | string | Country name |
| country_code | string | ISO country code (e.g., "id") |

---

#### Example: First Page

**Request:**
```
GET /api/v1/feed?latitude=-6.3653&longitude=106.8269&limit=10
```

**Response:** `200 OK`
```json
{
  "data": [
    {
      "id": "a6b4ff20-ea1b-11f0-879d-7a2e88169b55",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "username": "john_doe",
      "profile_picture_url": "http://localhost:8080/uploads/avatars/john.jpg",
      "content": "Beautiful morning in Jakarta! ☀️",
      "media_urls": ["http://localhost:8080/uploads/posts/abc.jpg"],
      "latitude": -6.3653,
      "longitude": 106.8269,
      "geohash": "qqguy",
      "created_at": "2026-01-05T10:30:00Z",
      "distance_km": 0.5
    }
  ],
  "count": 10,
  "has_more": true,
  "next_cursor": "MjAyNi0wMS0wNVQxMDozMDowMFo="
}
```

---

#### Example: Load More (Next Page)

**Request:**
```
GET /api/v1/feed?latitude=-6.3653&longitude=106.8269&limit=10&cursor=MjAyNi0wMS0wNVQxMDozMDowMFo=
```

**Response:** `200 OK`
```json
{
  "data": [/* next 10 posts */],
  "count": 10,
  "has_more": true,
  "next_cursor": "MjAyNi0wMS0wNFQxNTowMDowMFo="
}
```

---

#### Example: Last Page

**Response:** `200 OK`
```json
{
  "data": [/* final 3 posts */],
  "count": 3,
  "has_more": false,
  "next_cursor": ""
}
```

---

#### Mobile Implementation Pseudocode

```swift
// Swift/iOS Example
class FeedViewModel {
    var posts: [Post] = []
    var nextCursor: String? = nil
    var hasMore = true
    var isLoading = false
    
    func loadInitialFeed() async {
        nextCursor = nil
        posts = []
        await loadMore()
    }
    
    func loadMore() async {
        guard hasMore, !isLoading else { return }
        isLoading = true
        
        var url = "/api/v1/feed?latitude=\(lat)&longitude=\(lng)&limit=20"
        if let cursor = nextCursor {
            url += "&cursor=\(cursor)"
        }
        
        let response = await api.get(url)
        posts.append(contentsOf: response.data)
        nextCursor = response.next_cursor
        hasMore = response.has_more
        isLoading = false
    }
    
    // Call loadMore() when user scrolls near bottom
}
```

```kotlin
// Kotlin/Android Example
class FeedViewModel : ViewModel() {
    val posts = mutableListOf<Post>()
    var nextCursor: String? = null
    var hasMore = true
    var isLoading = false
    
    fun loadInitialFeed() {
        nextCursor = null
        posts.clear()
        loadMore()
    }
    
    fun loadMore() {
        if (!hasMore || isLoading) return
        isLoading = true
        
        viewModelScope.launch {
            val params = mutableMapOf(
                "latitude" to lat,
                "longitude" to lng,
                "limit" to 20
            )
            nextCursor?.let { params["cursor"] = it }
            
            val response = api.getFeed(params)
            posts.addAll(response.data)
            nextCursor = response.nextCursor
            hasMore = response.hasMore
            isLoading = false
        }
    }
}
```

---

## Protected Endpoints

> All endpoints below require `Authorization: Bearer <token>` header.

---

### Profile

#### Update Current User Profile

**Endpoint:** `PUT /api/v1/users/me`

**Request Body:**
| Field | Type | Description |
|-------|------|-------------|
| full_name | string | Display name |
| bio | string | User bio |
| phone_number | string | Phone number |
| profile_picture_url | string | Avatar URL |

**Success Response:** `200 OK`
```json
{
  "message": "Profile updated",
  "user": { ... }
}
```

---

### Users

#### Get User by ID

**Endpoint:** `GET /api/v1/users/:id`

**Success Response:** `200 OK`
```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "username": "johndoe",
    "full_name": "John Doe",
    "bio": "Tech enthusiast",
    "profile_picture_url": "http://...",
    "created_at": "2025-12-13T19:30:00Z"
  }
}
```

#### Get User by Username

**Endpoint:** `GET /api/v1/users/username/:username`

#### Get User's Posts

**Endpoint:** `GET /api/v1/users/:id/posts`

**Success Response:** `200 OK`
```json
{
  "count": 10,
  "posts": [ ... ]
}
```

---

### Follows

#### Follow User

**Endpoint:** `POST /api/v1/users/:id/follow`

**Success Response:** `200 OK`
```json
{
  "message": "User followed"
}
```

#### Unfollow User

**Endpoint:** `DELETE /api/v1/users/:id/follow`

**Success Response:** `200 OK`
```json
{
  "message": "User unfollowed"
}
```

#### Get Followers

**Endpoint:** `GET /api/v1/users/:id/followers`

**Query Parameters:**
| Parameter | Type | Default |
|-----------|------|---------|
| limit | int | 50 |

**Success Response:** `200 OK`
```json
{
  "user_id": "...",
  "count": 42,
  "followers": [ ... ]
}
```

#### Get Following

**Endpoint:** `GET /api/v1/users/:id/following`

**Query Parameters:**
| Parameter | Type | Default |
|-----------|------|---------|
| limit | int | 50 |

**Success Response:** `200 OK`
```json
{
  "user_id": "...",
  "count": 15,
  "following": [ ... ]
}
```

---

### Posts

#### Create Post

**Endpoint:** `POST /api/v1/posts`

**Request Body:**
| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| user_id | UUID | Yes | Must exist |
| content | string | Yes | - |
| latitude | float | Yes | -90 to 90 |
| longitude | float | Yes | -180 to 180 |
| media_urls | array | No | Max 4 URLs |

**Request Example:**
```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "content": "Hello from Central Park!",
  "latitude": 40.785091,
  "longitude": -73.968285,
  "media_urls": ["http://localhost:8080/uploads/posts/photo.jpg"]
}
```

**Success Response:** `201 Created`
```json
{
  "message": "Post created successfully",
  "post": { ... }
}
```

#### Get Post by ID

**Endpoint:** `GET /api/v1/posts/:id`

**Success Response:** `200 OK`
```json
{
  "post": { ... }
}
```

---

### Likes

#### Like Post

**Endpoint:** `POST /api/v1/posts/:id/like`

**Success Response:** `200 OK`
```json
{
  "message": "Post liked"
}
```

#### Unlike Post

**Endpoint:** `DELETE /api/v1/posts/:id/like`

**Success Response:** `200 OK`
```json
{
  "message": "Post unliked"
}
```

#### Like Comment

**Endpoint:** `POST /api/v1/comments/:id/like`

#### Unlike Comment

**Endpoint:** `DELETE /api/v1/comments/:id/like`

---

### Comments

#### Create Comment

**Endpoint:** `POST /api/v1/posts/:id/comments`

**Request Body:**
| Field | Type | Required |
|-------|------|----------|
| content | string | Yes |

**Success Response:** `201 Created`
```json
{
  "message": "Comment created",
  "comment": {
    "id": "...",
    "post_id": "...",
    "user_id": "...",
    "content": "Great post!",
    "depth": 1,
    "created_at": "..."
  }
}
```

#### Get Comments for Post

**Endpoint:** `GET /api/v1/posts/:id/comments`

**Query Parameters:**
| Parameter | Type | Default |
|-----------|------|---------|
| limit | int | 50 |

**Success Response:** `200 OK`
```json
{
  "post_id": "...",
  "total_count": 15,
  "comments": [ ... ]
}
```

#### Reply to Comment

**Endpoint:** `POST /api/v1/comments/:id/reply`

> Maximum nesting depth is 3 levels.

**Request Body:**
```json
{
  "content": "Thanks!"
}
```

**Success Response:** `201 Created`
```json
{
  "message": "Reply created",
  "comment": { ... }
}
```

#### Delete Comment

**Endpoint:** `DELETE /api/v1/comments/:id`

> Users can only delete their own comments.

**Success Response:** `200 OK`
```json
{
  "message": "Comment deleted"
}
```

---

### Locations

#### Follow Location

**Endpoint:** `POST /api/v1/locations/follow`

**Request Body:**
| Field | Type | Required |
|-------|------|----------|
| latitude | float | Yes |
| longitude | float | Yes |
| name | string | No |

**Request Example:**
```json
{
  "latitude": 40.785091,
  "longitude": -73.968285,
  "name": "Central Park"
}
```

**Success Response:** `201 Created`
```json
{
  "message": "Location followed",
  "location": {
    "geohash_prefix": "dr5ru",
    "name": "Central Park",
    "latitude": 40.785091,
    "longitude": -73.968285
  }
}
```

#### Unfollow Location

**Endpoint:** `DELETE /api/v1/locations/:geohash/follow`

**Success Response:** `200 OK`
```json
{
  "message": "Location unfollowed"
}
```

#### Get Followed Locations

**Endpoint:** `GET /api/v1/locations/following`

**Success Response:** `200 OK`
```json
{
  "locations": [ ... ],
  "count": 3
}
```

---

### Notifications

#### Get Notifications

**Endpoint:** `GET /api/v1/notifications`

**Query Parameters:**
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| limit | int | 50 | Max results |
| unread | bool | false | Filter unread only |

**Success Response:** `200 OK`
```json
{
  "notifications": [
    {
      "id": "...",
      "type": "follow",
      "actor_id": "...",
      "message": "started following you",
      "is_read": false,
      "created_at": "..."
    }
  ],
  "unread_count": 5,
  "total": 20
}
```

#### Mark Notification as Read

**Endpoint:** `PUT /api/v1/notifications/:id/read`

**Success Response:** `200 OK`
```json
{
  "message": "Notification marked as read"
}
```

#### Mark All Notifications as Read

**Endpoint:** `PUT /api/v1/notifications/read-all`

**Success Response:** `200 OK`
```json
{
  "message": "All notifications marked as read"
}
```

---

### Search

#### Search Users

**Endpoint:** `GET /api/v1/search/users`

**Query Parameters:**
| Parameter | Type | Required | Constraints |
|-----------|------|----------|-------------|
| q | string | Yes | Min 2 characters |
| limit | int | No | Default: 20 |

**Success Response:** `200 OK`
```json
{
  "query": "john",
  "results": [ ... ],
  "count": 5
}
```

#### Search Posts

**Endpoint:** `GET /api/v1/search/posts`

**Query Parameters:**
| Parameter | Type | Required | Constraints |
|-----------|------|----------|-------------|
| q | string | Yes | Min 2 characters |
| limit | int | No | Default: 20 |

**Success Response:** `200 OK`
```json
{
  "query": "central park",
  "results": [ ... ],
  "count": 10
}
```

---

### Upload

#### Upload Avatar

**Endpoint:** `POST /api/v1/upload/avatar`

**Content-Type:** `multipart/form-data`

| Field | Type | Constraints |
|-------|------|-------------|
| file | file | Max 5MB, JPEG/PNG/GIF/WebP |

**Success Response:** `200 OK`
```json
{
  "message": "Avatar uploaded",
  "filename": "avatars/abc123.jpg",
  "url": "http://localhost:8080/uploads/avatars/abc123.jpg"
}
```

#### Upload Post Media

**Endpoint:** `POST /api/v1/upload/post`

**Content-Type:** `multipart/form-data`

| Field | Type | Constraints |
|-------|------|-------------|
| file | file | Max 50MB, JPEG/PNG/GIF/WebP/MP4/MOV |

**Success Response:** `200 OK`
```json
{
  "message": "Media uploaded",
  "filename": "posts/xyz789.mp4",
  "url": "http://localhost:8080/uploads/posts/xyz789.mp4",
  "media_type": "video",
  "extension": ".mp4"
}
```

---

### Devices

#### Register Device (Push Notifications)

**Endpoint:** `POST /api/v1/devices`

**Request Body:**
```json
{
  "token": "fcm_device_token_here",
  "platform": "ios"
}
```

**Success Response:** `200 OK`
```json
{
  "message": "Device registered"
}
```

#### Unregister Device

**Endpoint:** `DELETE /api/v1/devices`

**Success Response:** `200 OK`
```json
{
  "message": "Device unregistered"
}
```

---

## Error Responses

All errors follow this format:

```json
{
  "error": "Error message",
  "details": "Optional detailed information"
}
```

### Common HTTP Status Codes

| Code | Description |
|------|-------------|
| 200 | Success |
| 201 | Created |
| 400 | Bad Request (validation failed) |
| 401 | Unauthorized (invalid/missing token) |
| 403 | Forbidden (not allowed) |
| 404 | Not Found |
| 409 | Conflict (duplicate resource) |
| 429 | Too Many Requests (rate limited) |
| 500 | Internal Server Error |

---

## Rate Limits

| Limit | Value |
|-------|-------|
| Requests per IP | 100/minute |

When rate limited, you'll receive:

```json
{
  "error": "Rate limit exceeded. Please try again later."
}
```

---

## CORS Configuration

| Setting | Value |
|---------|-------|
| Allow Origins | `*` |
| Allow Methods | GET, POST, PUT, DELETE |
| Allow Headers | Origin, Content-Type, Authorization |

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CASSANDRA_HOST` | `localhost` | Cassandra host |
| `CASSANDRA_KEYSPACE` | `geoloc` | Keyspace name |
| `JWT_SECRET` | (default) | JWT signing secret |
| `PORT` | `8080` | Server port |
| `UPLOAD_PATH` | `./uploads` | Upload directory |
| `BASE_URL` | `http://localhost:8080` | Base URL for uploads |
