# Frontend integration guide — Notifications list (Phase 1)

**Audience:** Frontend / mobile AI agent implementing in-app notifications.  
**Scope (this phase):** `GET /api/v1/notifications` only.  
**Out of scope for now:** SSE stream, mark read, push/FCM, delete.

---

## Goal

Wire the **existing notifications UI** to `GET /api/v1/notifications`: load data, map fields to list rows, drive unread/badge state, and handle tap navigation. Do not redesign or rebuild the screen layout.

---

## API contract

### Request

```http
GET {baseUrl}/api/v1/notifications?limit=50
Authorization: Bearer {accessToken}
```

| Query param | Type | Default | Max | Description |
|-------------|------|---------|-----|-------------|
| `limit` | number | `50` | `100` | Max items returned |
| `unread` | `"true"` | — | — | If `unread=true`, only unread notifications are returned |

**Examples:**

```http
GET /api/v1/notifications?limit=50
GET /api/v1/notifications?limit=20&unread=true
```

### Authentication

- **Required:** `Authorization: Bearer <access_token>` from login/register/refresh.
- Access tokens expire after **15 minutes**. On `401` with `"Token has expired"`, refresh via `POST /auth/refresh` or re-login, then retry.

### Success response `200 OK`

```json
{
  "notifications": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "user_id": "recipient-user-uuid",
      "type": "like",
      "actor_id": "actor-user-uuid",
      "target_type": "post",
      "target_id": "post-uuid",
      "message": "liked your post",
      "payload": {
        "post_preview": "Hello from Jakarta..."
      },
      "is_read": false,
      "created_at": "2026-05-14T10:30:00Z"
    }
  ],
  "unread_count": 5,
  "total": 1
}
```

| Field | Type | Notes |
|-------|------|--------|
| `notifications` | array | Newest first (server-ordered by `created_at` desc) |
| `unread_count` | number | Total unread for current user (all notifications, not just this page) |
| `total` | number | Length of `notifications` array in **this** response |

**Important:** There is **no** nested `actor` object. Use `actor_id` to load avatar/username (e.g. existing `GET /api/v1/users/:id` or cache).

**Important:** There is **no cursor pagination** on this endpoint. Only `limit`. For “load more”, increase `limit` or wait for a future API version.

### Error responses

| Status | Body | Client action |
|--------|------|----------------|
| `401` | `{ "error": "..." }` | Refresh token / login |
| `500` | `{ "error": "..." }` | Show error, allow retry |

---

## Notification `type` values

| `type` | Meaning | Suggested navigation |
|--------|---------|-------------------|
| `like` | Someone liked your post | Post detail: `target_id` when `target_type` is `post` |
| `comment` | Someone commented on your post | Post detail (comments): `target_id` |
| `follow` | Someone followed you | User profile: `actor_id` |
| `location_post` | New post in an area you follow | Post detail: `target_id` |

`target_type` / `target_id` may be empty for some events; fall back to `actor_id` profile.

`payload` is optional `Record<string, string>` (e.g. `post_preview`, `title` from push pipeline). Use for subtitle text only; do not trust for routing without `target_type` + `target_id`.

---

## Recommended TypeScript types

```typescript
export type NotificationType =
  | "like"
  | "comment"
  | "follow"
  | "location_post";

export interface AppNotification {
  id: string;
  user_id: string;
  type: NotificationType;
  actor_id: string;
  target_type?: string;
  target_id?: string;
  message: string;
  payload?: Record<string, string>;
  is_read: boolean;
  created_at: string; // ISO 8601
}

export interface NotificationsListResponse {
  notifications: AppNotification[];
  unread_count: number;
  total: number;
}
```

---

## Implementation checklist (wire existing UI)

### 1. API client function

```typescript
async function fetchNotifications(
  accessToken: string,
  options?: { limit?: number; unreadOnly?: boolean }
): Promise<NotificationsListResponse> {
  const params = new URLSearchParams();
  params.set("limit", String(options?.limit ?? 50));
  if (options?.unreadOnly) params.set("unread", "true");

  const res = await fetch(
    `${BASE_URL}/api/v1/notifications?${params}`,
    {
      headers: {
        Authorization: `Bearer ${accessToken}`,
        Accept: "application/json",
      },
    }
  );

  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? `HTTP ${res.status}`);
  }

  return res.json();
}
```

### 2. Bind response to existing list

Map API fields to whatever the current row component already expects:

| API field | Use in existing UI |
|-----------|-------------------|
| `notifications` | List `data` / `items` source |
| `unread_count` | Badge count (existing badge component) |
| `is_read` | Unread indicator / styling already in design |
| `message` | Primary line text (prefer over client templates) |
| `payload.post_preview` | Secondary line / preview if you have one |
| `created_at` | Relative timestamp |
| `actor_id` | Fetch avatar + display name (see below) |

On load / refresh / focus: call `fetchNotifications` with `limit=50` and replace list state. Reuse existing empty, loading, and error UI — only swap in real API data and errors.

### 3. Display copy (if not using raw `message`)

| `type` | Template |
|--------|----------|
| `like` | `{username} liked your post` |
| `comment` | `{username} commented on your post` |
| `follow` | `{username} started following you` |
| `location_post` | `New post near {location}` or use `message` |

Prefer server `message` when present; use templates only as fallback.

### 4. Actor resolution

Batch unique `actor_id` values from the list, then:

```http
GET /api/v1/users/{actor_id}
```

Cache results in memory for the session to avoid N+1 on scroll.

### 5. Tap navigation

```typescript
function onNotificationPress(n: AppNotification) {
  switch (n.type) {
    case "follow":
      navigateToUserProfile(n.actor_id);
      break;
    case "like":
    case "comment":
    case "location_post":
      if (n.target_id && n.target_type === "post") {
        navigateToPost(n.target_id);
      }
      break;
  }
  // Phase 2: call PUT .../notifications/:id/read after tap
}
```

### 6. State management (minimal)

Extend existing notifications state/store (do not introduce a parallel pattern unless needed):

- `notifications: AppNotification[]`
- `unread_count: number`
- `isLoading`, `error` (if not already present)

On mount / focus / pull-to-refresh (whatever the app already uses): fetch with `limit=50`, then update list + badge from the response.

---

## What NOT to implement in Phase 1

| Feature | Endpoint | Phase |
|---------|----------|-------|
| Mark one read | `PUT /api/v1/notifications/:id/read` | 2 |
| Mark all read | `PUT /api/v1/notifications/read-all` | 2 |
| Unread count only | `GET /api/v1/notifications/unread-count` | 2 (optional; already in list response) |
| Real-time updates | `GET /api/v1/notifications/stream` (SSE) | 2 |
| Push (FCM) | `POST /api/v1/devices` | 3 |

---

## Testing against local API

1. `baseUrl`: `http://localhost:8080` (or device LAN IP for emulator).
2. Login user A → save `accessToken`.
3. Login user B → follow user A or toggle-like user A’s post (`POST /api/v1/posts/:id/toggle-like`, empty body).
4. As user A: `GET /api/v1/notifications?limit=50` → expect at least one item.

---

## Acceptance criteria (Phase 1)

- [ ] Existing notifications list shows live data from `GET /api/v1/notifications?limit=50`.
- [ ] `unread_count` drives the existing badge.
- [ ] `is_read` drives existing unread styling.
- [ ] Row tap uses existing navigation to post or profile per `type`.
- [ ] Existing refresh path refetches from API.
- [ ] Token expiry (`401`) handled with existing auth refresh flow.

---

## Reference

- Backend handler: `internal/handlers/notification.go` (`GetNotifications`)
- Model: `internal/data/models.go` (`Notification`)
- Full notifications API (later phases): [docs/api/notifications.md](../api/notifications.md)
