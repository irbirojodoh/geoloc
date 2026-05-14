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

## SSE Real-Time Stream

**Endpoint:** `GET /api/v1/notifications/stream`

> ⚠️ **Requires Authentication** — Uses SSE (Server-Sent Events) for real-time notification delivery.

Connects to a persistent SSE stream that pushes notifications as they happen.

**Headers:**
- `Authorization: Bearer <access_token>`
- `Accept: text/event-stream`

**Event Format:**
```
event: connected
data: {"status":"connected","time":"2026-01-05T10:30:00Z"}

event: notification
data: { ... notification JSON ... }

event: :keepalive
data: ...
```

**Heartbeat:** A keepalive event is sent every 30 seconds.

**Clients should:**
- Auto-reconnect on connection drop (exponential backoff recommended)
- Parse `event:` lines to differentiate notification types

## Get Unread Count

**Endpoint:** `GET /api/v1/notifications/unread-count`

**Response:** `200 OK`
```json
{
  "unread_count": 5
}
```

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

## Delete Notification

**Endpoint:** `DELETE /api/v1/notifications/:id`

**Response:** `200 OK`
```json
{
  "message": "Notification deleted"
}
```

---

## Device registration (push)

**Register:** `POST /api/v1/devices`

```json
{
  "token": "fcm-registration-token",
  "platform": "ios"
}
```

`platform` must be `ios`, `android`, or `web`.

**Unregister:** `DELETE /api/v1/devices` with body `{ "token": "..." }`.

Tokens are stored in Cassandra `push_device_tokens` (see migration `006_notifications_v2.cql`).

---

## What triggers notifications

| Event | Endpoint | Notes |
|-------|----------|-------|
| Follow | `POST /api/v1/users/:id/follow` | Always dispatches |
| Post like | `POST /api/v1/posts/:id/toggle-like` | Only when `changed: true` and `is_liked: true`; **not** legacy `POST .../like` |
| Comment | `POST /api/v1/posts/:id/comments` | Comment notification |
| Nearby post | Post create + location followers | Via Kafka nearby fanout |

Access tokens expire after **15 minutes** — refresh or re-login before testing.

---

## Local testing

See [Push notification testing](../testing-push-notifications.md) for Level 1 (log-only) and Level 2 (real FCM web push) with Postman.
