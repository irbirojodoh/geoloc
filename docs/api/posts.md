# Posts API

Endpoints for creating, reading, and interacting with posts.

## Create Post

**Endpoint:** `POST /api/v1/posts`

**Request:**
```json
{
  "content": "Beautiful sunset! 🌅",
  "media_urls": ["https://example.com/upload.jpg"],
  "latitude": -6.3653,
  "longitude": 106.8269
}
```

**Response:** `201 Created`
```json
{
  "id": "a6b4ff20-ea1b-11f0-879d-7a2e88169b55",
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "content": "Beautiful sunset! 🌅",
  "media_urls": ["https://example.com/upload.jpg"],
  "geohash": "qqggy",
  "created_at": "2026-01-05T10:30:00Z"
}
```

## Get Post

**Endpoint:** `GET /api/v1/posts/:id`

**Response:** `200 OK`
```json
{
  "post": {
    "id": "a6b4ff20-ea1b-11f0-879d-7a2e88169b55",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "content": "Beautiful sunset! 🌅",
    "media_urls": ["https://example.com/upload.jpg"],
    "geohash": "qqggy",
    "created_at": "2026-01-05T10:30:00Z"
  }
}
```

## Like Post

**Endpoint:** `POST /api/v1/posts/:id/like`

**Response:** `200 OK`
```json
{
  "message": "Post liked"
}
```

## Unlike Post

**Endpoint:** `DELETE /api/v1/posts/:id/like`

**Response:** `200 OK`
```json
{
  "message": "Post unliked"
}
```

## Upload Post Media

Upload images/videos before creating a post.

**Endpoint:** `POST /api/v1/upload/post`

**Request:** `multipart/form-data`
- `file`: Image or video file (max 50MB)

**Response:** `200 OK`
```json
{
  "url": "http://localhost:8080/uploads/posts/abc123.jpg"
}
```

## Media Constraints

| Type | Max Size | Max Count |
|------|----------|-----------|
| Images | 50MB each | 4 per post |
| Videos | 50MB each | 1 per post |
