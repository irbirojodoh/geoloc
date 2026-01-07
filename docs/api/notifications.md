# Notifications API

Endpoints for in-app notifications.

## Get Notifications

**Endpoint:** `GET /api/v1/notifications`

**Query Parameters:**
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | int | 50 | Max notifications |

**Response:** `200 OK`
```json
{
  "count": 3,
  "notifications": [
    {
      "id": "notif-uuid",
      "type": "like",
      "actor_id": "user-uuid",
      "target_type": "post",
      "target_id": "post-uuid",
      "message": "john_doe liked your post",
      "is_read": false,
      "created_at": "2026-01-05T10:30:00Z",
      "actor": {
        "id": "user-uuid",
        "username": "john_doe",
        "profile_picture_url": "..."
      }
    }
  ]
}
```

## Notification Types

| Type | Description |
|------|-------------|
| `like` | Someone liked your post |
| `comment` | Someone commented on your post |
| `follow` | Someone followed you |
| `location_post` | New post in followed location |

## Mark as Read

**Endpoint:** `PUT /api/v1/notifications/:id/read`

**Response:** `200 OK`
```json
{
  "message": "Notification marked as read"
}
```

## Mark All as Read

**Endpoint:** `PUT /api/v1/notifications/read-all`

**Response:** `200 OK`
```json
{
  "message": "All notifications marked as read"
}
```
