# API Overview

The Geoloc API is a RESTful JSON API for a hyper-local social media platform.

## Base URL

```
Development: http://localhost:8080
Production:  https://api.yourapp.com
```

## Authentication

Most endpoints require JWT authentication via Bearer token:

```
Authorization: Bearer <access_token>
```

See [Authentication](./authentication.md) for details on obtaining tokens.

## Public Endpoints

| Endpoint | Description |
|----------|-------------|
| `POST /auth/register` | Create new account |
| `POST /auth/login` | Login and get tokens |
| `POST /auth/refresh` | Refresh access token |
| `GET /health` | Health check |

## Protected Endpoints

All `/api/v1/*` endpoints require authentication.

| Category | Endpoints |
|----------|-----------|
| [Feed](./feed.md) | `GET /api/v1/feed` |
| [Posts](./posts.md) | `POST /api/v1/posts`, `GET /api/v1/posts/:id`, etc. |
| [Users](./users.md) | `GET /api/v1/users/:id`, `PUT /api/v1/users/me`, etc. |
| [Comments](./comments.md) | `POST /api/v1/posts/:id/comments`, etc. |
| [Notifications](./notifications.md) | `GET /api/v1/notifications`, etc. |
| [Search](./search.md) | `GET /api/v1/search/users`, etc. |

## Response Format

### Success Response

```json
{
  "data": [...],
  "count": 10,
  "has_more": true,
  "next_cursor": "base64cursor"
}
```

### Error Response

```json
{
  "error": "Error message",
  "details": "Additional context"
}
```

## Pagination

All list endpoints use **cursor-based pagination**:

| Parameter | Description |
|-----------|-------------|
| `limit` | Items per page (default: 20, max: 100) |
| `cursor` | Base64-encoded cursor from previous response |

**Flow:**
1. First request: Omit `cursor`
2. Check `has_more` in response
3. If `true`, use `next_cursor` for next page
4. Repeat until `has_more` is `false`

## Rate Limiting

- **Global**: 100 requests/minute per IP
- **Nominatim geocoding**: 1 request/second (handled server-side)

When rate limited, you'll receive `429 Too Many Requests`.

## Privacy

Posts do not expose precise coordinates. Only `geohash` (approximate location) is returned for user privacy.
