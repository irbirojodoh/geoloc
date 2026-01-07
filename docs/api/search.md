# Search API

Endpoints for searching users and posts.

## Search Users

**Endpoint:** `GET /api/v1/search/users`

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `q` | string | Yes | Search query |
| `limit` | int | No | Max results (default: 20) |

**Response:** `200 OK`
```json
{
  "count": 2,
  "users": [
    {
      "id": "user-uuid",
      "username": "john_doe",
      "full_name": "John Doe",
      "profile_picture_url": "..."
    }
  ]
}
```

## Search Posts

**Endpoint:** `GET /api/v1/search/posts`

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `q` | string | Yes | Search query |
| `limit` | int | No | Max results (default: 20) |

**Response:** `200 OK`
```json
{
  "count": 5,
  "posts": [
    {
      "id": "post-uuid",
      "content": "Matching content...",
      "geohash": "qqggy",
      "created_at": "2026-01-05T10:30:00Z"
    }
  ]
}
```

## Best Practices

- **Debounce**: Wait 300-500ms after user stops typing before searching
- **Min Length**: Require at least 2-3 characters before searching
- **Caching**: Cache results for repeated queries
