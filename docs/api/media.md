# Media & Upload API

This document describes the API endpoints for uploading, deleting, and securely serving media assets (avatars, cover images, and post images) in Geoloc.

All uploaded media is stored privately in Cloudflare R2 and served securely through the Go API.

---

## 1. Protected Serving Endpoint

### Serve Private File
All media key references returned in user profiles and post payloads are resolved to this proxy route. The server checks user authentication first, then downloads and streams the file from Cloudflare R2 on the fly.

**Endpoint:** `GET /api/v1/media/file`

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `key` | string | Yes | The object key, e.g., `posts/user-123/uuid.jpg` |

**Headers Forwarded by Server:**
- `Content-Type`: Automatically set based on the original file mime-type.
- `Content-Length`: Automatically set to the original file size in bytes.
- `Cache-Control`: `public, max-age=86400` (24-hour browser caching).

**Response:** `200 OK` (Streams binary data)

**Error Responses:**
- `400 Bad Request`: If the key fails traversal checks or uses an invalid prefix.
- `401 Unauthorized`: Missing or invalid Bearer token.
- `404 Not Found`: File does not exist in the bucket.

---

## 2. Server-Side Uploads (Pattern A)

These endpoints accept file uploads directly from the client via multipart form posts, validate the file size and type, and upload them directly to R2.

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
  "url": "http://localhost:8080/api/v1/media/file?key=avatars/user-123/550e8400-e29b-41d4-a716-446655440000.jpg"
}
```

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
  "url": "http://localhost:8080/api/v1/media/file?key=covers/user-123/550e8400-e29b-41d4-a716-446655440000.jpg"
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
  "url": "http://localhost:8080/api/v1/media/file?key=posts/user-123/550e8400-e29b-41d4-a716-446655440000.jpg"
}
```

---

## 3. Client-Side Direct Uploads (Pattern B)

For uploading files directly to R2 from mobile or web clients without proxying the upload bandwidth through the Go server.

### Request Presigned Upload URL
Requests a temporary (10-minute validity) presigned PUT URL to upload files directly to Cloudflare R2.

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

**Next Step for Client:**
Issue a raw `PUT` request containing the binary payload directly to the `upload_url`. Be sure to set the `Content-Type` header to exactly match the requested `content_type`.

---

## 4. Deletion

### Delete Object
Deletes a media asset from R2. The server validates that the active user owns the file by verifying that the `userID` in the request context matches the `userID` segment embedded in the key.

**Endpoint:** `DELETE /api/v1/media/object`

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `key` | string | Yes | The object key to delete, e.g. `posts/user-123/uuid.jpg` |

**Response:** `200 OK`
```json
{
  "message": "Object deleted"
}
```

**Error Responses:**
- `403 Forbidden`: If the user attempts to delete an asset owned by another user.
