# Feed API

The feed endpoint returns posts near a geographic location with cursor-based pagination.

## Get Feed

**Endpoint:** `GET /api/v1/feed`

> ⚠️ **Requires Authentication**

### Query Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `latitude` | float | Yes | - | Latitude (-90 to 90) |
| `longitude` | float | Yes | - | Longitude (-180 to 180) |
| `radius_km` | float | No | 10 | Search radius in kilometers |
| `limit` | int | No | 20 | Posts per page (max 100) |
| `cursor` | string | No | - | Pagination cursor |

### Response

```json
{
  "data": [
    {
      "id": "a6b4ff20-ea1b-11f0-879d-7a2e88169b55",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "username": "john_doe",
      "profile_picture_url": "https://example.com/avatar.jpg",
      "content": "Beautiful morning in Jakarta! ☀️",
      "media_urls": ["https://example.com/image.jpg"],
      "geohash": "qqggy",
      "location_name": "Kukusan",
      "address": {
        "village": "Kukusan",
        "city_district": "Beji",
        "city": "Depok",
        "state": "West Java",
        "country": "Indonesia",
        "country_code": "id"
      },
      "created_at": "2026-01-05T10:30:00Z",
      "distance_km": 0.5
    }
  ],
  "count": 10,
  "has_more": true,
  "next_cursor": "MjAyNi0wMS0wNVQxMDozMDowMFo="
}
```

### Post Fields

| Field | Description |
|-------|-------------|
| `id` | Post UUID |
| `user_id` | Author's UUID |
| `username` | Author's username |
| `profile_picture_url` | Author's avatar |
| `content` | Post text |
| `media_urls` | Array of media URLs (max 4) |
| `geohash` | Approximate location (5-char geohash) |
| `location_name` | Place name (e.g., "Kukusan") |
| `address` | Full address object |
| `created_at` | ISO 8601 timestamp |
| `distance_km` | Distance from query location |

### Address Object

| Field | Description |
|-------|-------------|
| `village` | Village/neighborhood |
| `city_district` | District within city |
| `city` | City name |
| `state` | State/province |
| `country` | Country name |
| `country_code` | ISO country code |

### Example Usage

**First page:**
```bash
curl "http://localhost:8080/api/v1/feed?latitude=-6.3653&longitude=106.8269&limit=10" \
  -H "Authorization: Bearer <token>"
```

**Next page:**
```bash
curl "http://localhost:8080/api/v1/feed?latitude=-6.3653&longitude=106.8269&limit=10&cursor=MjAyNi0wMS0wNQ==" \
  -H "Authorization: Bearer <token>"
```

### Privacy Note

Posts do not return precise coordinates (`latitude`, `longitude`). Only `geohash` is returned to protect user location privacy.
