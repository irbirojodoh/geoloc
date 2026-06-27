# Media & Upload API

This document describes the API endpoints for uploading, deleting, and serving media assets (avatars, cover images, and post images) in Geoloc.

All uploaded media is stored in a **private** Cloudflare R2 bucket. The database stores object **keys** only (e.g. `posts/user-123/uuid.jpg`). API responses resolve those keys to **presigned GET URLs** so clients fetch media **directly from R2** without proxying bytes through the Go API.

---

## 1. Serving Media (Presigned GET URLs)

### Automatic resolution in API responses

Whenever a handler returns a stored R2 key in fields such as `profile_picture_url`, `cover_image_url`, or `media_urls`, the server replaces the key with a **presigned GET URL** pointing at R2.

| Property | Value |
|----------|-------|
| Validity | **15 minutes** (`PresignGetExpiry`) |
| Client fetch | Direct `GET` to R2 — no API middleware on the download |
| External URLs | Passed through unchanged (legacy posts, default cover image) |

**Example** — feed/post/profile payload:

```json
{
  "profile_picture_url": "https://<account-id>.r2.cloudflarestorage.com/geoloc-media/avatars/user-123/uuid.jpg?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=...&X-Amz-Date=...&X-Amz-Expires=900&X-Amz-SignedHeaders=host&X-Amz-Signature=...",
  "media_urls": [
    "https://<account-id>.r2.cloudflarestorage.com/geoloc-media/posts/user-123/uuid.jpg?X-Amz-Algorithm=..."
  ]
}
```

**Client guidance:**

- Use presigned URLs soon after receiving them; do not cache them beyond their TTL.
- When a URL expires, refetch the parent resource (feed, post, profile) or call `GET /api/v1/media/sign` to obtain a fresh URL for a known key.

### Request presigned GET URL (on demand)

Explicitly sign a single object key. Useful when refreshing an expired URL without reloading an entire feed.

**Endpoint:** `GET /api/v1/media/sign`

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `key` | string | Yes | Object key, e.g. `posts/user-123/uuid.jpg` |

**Response:** `200 OK`

```json
{
  "url": "https://<account-id>.r2.cloudflarestorage.com/geoloc-media/posts/user-123/uuid.jpg?X-Amz-Algorithm=...",
  "expires_at": "2026-06-27T13:00:00Z"
}
```

**Error Responses:**

- `400 Bad Request` — Invalid or disallowed key.
- `401 Unauthorized` — Missing or invalid Bearer token.
- `500 Internal Server Error` — R2 presign failure.

### Legacy proxy endpoint (deprecated)

`GET /api/v1/media/file?key=...` still streams an object through the Go API for backward compatibility. **New clients should use presigned URLs** returned in API responses or from `/media/sign`.

---

## 2. Server-Side Uploads (Pattern A)

These endpoints accept file uploads via multipart form posts, validate size and type, and upload directly to R2.

### Upload Avatar

**Endpoint:** `POST /api/v1/upload/avatar`  
**Content-Type:** `multipart/form-data`  
**Request Payload:**

- `file`: Binary image file (JPEG, PNG, GIF, WebP). Max size **10MB**.

**Response:** `200 OK`

```json
{
  "message": "Avatar uploaded",
  "key": "avatars/user-123/550e8400-e29b-41d4-a716-446655440000.jpg",
  "url": "https://<account-id>.r2.cloudflarestorage.com/geoloc-media/avatars/user-123/550e8400-e29b-41d4-a716-446655440000.jpg?X-Amz-Algorithm=..."
}
```

The `url` is a presigned GET URL (15-minute validity). Persist the `key` when updating the profile (`avatar_key` on `PUT /users/me`).

### Upload Cover Image

**Endpoint:** `POST /api/v1/upload/cover`  
**Content-Type:** `multipart/form-data`  
**Request Payload:**

- `file`: Binary image file (JPEG, PNG, GIF, WebP). Max size **10MB**.

**Response:** `200 OK`

```json
{
  "message": "Cover image uploaded",
  "key": "covers/user-123/550e8400-e29b-41d4-a716-446655440000.jpg",
  "url": "https://<account-id>.r2.cloudflarestorage.com/geoloc-media/covers/user-123/550e8400-e29b-41d4-a716-446655440000.jpg?X-Amz-Algorithm=..."
}
```

### Upload Post Media

**Endpoint:** `POST /api/v1/upload/post`  
**Content-Type:** `multipart/form-data`  
**Request Payload:**

- `file`: Binary image file (JPEG, PNG, GIF, WebP). Max size **10MB**.

**Response:** `200 OK`

```json
{
  "message": "Media uploaded",
  "key": "posts/user-123/550e8400-e29b-41d4-a716-446655440000.jpg",
  "url": "https://<account-id>.r2.cloudflarestorage.com/geoloc-media/posts/user-123/550e8400-e29b-41d4-a716-446655440000.jpg?X-Amz-Algorithm=..."
}
```

---

## 3. Client-Side Direct Uploads (Pattern B)

For uploading files directly to R2 from mobile or web clients without proxying upload bandwidth through the Go server.

### Request presigned upload URL

Requests a temporary (**10-minute**) presigned PUT URL to upload files directly to Cloudflare R2.

**Endpoint:** `POST /api/v1/media/upload-url`  
**Content-Type:** `application/json`  
**Request Body:**

```json
{
  "folder": "posts",
  "content_type": "image/png",
  "filename": "pic.png"
}
```

**Response:** `200 OK`

```json
{
  "upload_url": "https://<account-id>.r2.cloudflarestorage.com/geoloc-media/posts/user-123/uuid.png?X-Amz-Algorithm=...",
  "key": "posts/user-123/550e8400-e29b-41d4-a716-446655440000.png",
  "expires_at": "2026-06-16T12:45:00Z"
}
```

**Next step for client:**

Issue a raw `PUT` request with the binary payload to `upload_url`. Set the `Content-Type` header to exactly match the requested `content_type`.

Then attach the returned `key` via `media_keys` on post creation, or `avatar_key` / `cover_key` on profile update.

---

## 4. Deletion

### Delete Object

Deletes a media asset from R2. The server verifies that the authenticated user's ID matches the `userID` segment embedded in the key.

**Endpoint:** `DELETE /api/v1/media/object`

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `key` | string | Yes | Object key to delete, e.g. `posts/user-123/uuid.jpg` |

**Response:** `200 OK`

```json
{
  "message": "Object deleted"
}
```

**Error Responses:**

- `403 Forbidden` — User does not own the object.
- `401 Unauthorized` — Missing or invalid Bearer token.

---

## Key format & constraints

| Property | Value |
|----------|-------|
| Key pattern | `{folder}/{userId}/{uuid}{ext}` |
| Folders | `avatars`, `covers`, `posts` |
| Max file size | 10 MB |
| Allowed types | JPEG, PNG, GIF, WebP |
| Max per post | 4 images (`media_urls` + `media_keys` combined) |

---

## Client implementation

See [Media frontend guide](../client/media-frontend.md) for upload flows, attaching keys, displaying presigned URLs, and handling expiry in mobile/web apps.
