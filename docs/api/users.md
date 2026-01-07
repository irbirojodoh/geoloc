# Users API

Endpoints for user profiles and the follow system.

## Get User Profile

**Endpoint:** `GET /api/v1/users/:id`

**Response:** `200 OK`
```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "username": "john_doe",
    "email": "john@example.com",
    "full_name": "John Doe",
    "bio": "Developer from Jakarta",
    "profile_picture_url": "https://example.com/avatar.jpg",
    "followers_count": 150,
    "following_count": 75,
    "created_at": "2026-01-01T00:00:00Z"
  }
}
```

## Get User by Username

**Endpoint:** `GET /api/v1/users/username/:username`

Same response as Get User Profile.

## Update Profile

**Endpoint:** `PUT /api/v1/users/me`

**Request:**
```json
{
  "full_name": "John Smith",
  "bio": "Updated bio",
  "phone_number": "+1234567890"
}
```

**Response:** `200 OK`
```json
{
  "message": "Profile updated",
  "user": { ... }
}
```

## Upload Avatar

**Endpoint:** `POST /api/v1/upload/avatar`

**Request:** `multipart/form-data`
- `file`: Image file (max 5MB)

**Response:** `200 OK`
```json
{
  "url": "http://localhost:8080/uploads/avatars/abc123.jpg"
}
```

## Get User's Posts

**Endpoint:** `GET /api/v1/users/:id/posts`

Uses cursor-based pagination.

**Query Parameters:**
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | int | 20 | Posts per page |
| `cursor` | string | - | Pagination cursor |

**Response:** `200 OK`
```json
{
  "user": {
    "id": "550e8400-...",
    "username": "john_doe",
    "full_name": "John Doe",
    "profile_picture_url": "https://..."
  },
  "count": 10,
  "data": [
    {
      "id": "a6b4ff20-...",
      "content": "Post content",
      "geohash": "qqggy",
      "location_name": "Kukusan",
      "address": { ... }
    }
  ],
  "has_more": true,
  "next_cursor": "..."
}
```

---

## Follow System

### Follow User

**Endpoint:** `POST /api/v1/users/:id/follow`

**Response:** `200 OK`
```json
{
  "message": "User followed"
}
```

### Unfollow User

**Endpoint:** `DELETE /api/v1/users/:id/follow`

**Response:** `200 OK`
```json
{
  "message": "User unfollowed"
}
```

### Get Followers

**Endpoint:** `GET /api/v1/users/:id/followers`

**Response:** `200 OK`
```json
{
  "count": 2,
  "followers": [
    {
      "id": "...",
      "username": "follower1",
      "profile_picture_url": "..."
    }
  ]
}
```

### Get Following

**Endpoint:** `GET /api/v1/users/:id/following`

Same format as Get Followers.
