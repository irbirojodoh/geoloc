# Frontend integration guide — Media (upload & display)

**Audience:** Mobile / web client developers (Flutter-first examples).  
**Backend reference:** [Media & Upload API](../api/media.md)

---

## Goal

Implement avatar, cover, and post image flows so the client:

1. **Uploads** images to R2 (via API or presigned PUT).
2. **Attaches** uploads using object **keys** — not presigned URLs.
3. **Displays** images using presigned GET URLs returned by the API.
4. **Handles expiry** of presigned URLs without broken thumbnails after backgrounding or offline cache.

---

## Mental model

```
┌─────────────┐     JWT      ┌──────────┐     SDK      ┌─────────────┐
│   Client    │ ──────────►  │  Go API  │ ──────────►  │ Cloudflare  │
│             │              │          │              │     R2      │
└─────────────┘              └──────────┘              └─────────────┘
       │                            │
       │  Upload (Pattern A/B)      │  Stores key in Cassandra
       │  Attach key on profile/post│  e.g. posts/user-id/uuid.jpg
       │                            │
       │  Read feed/profile/post    │  Resolves key → presigned GET URL
       │ ◄──────────────────────────│  (15 min TTL)
       │
       │  GET presigned URL directly (no JWT)
       └──────────────────────────────────────────────────► R2
```

| Concept | Client responsibility |
|---------|----------------------|
| **Object key** | Stable identifier. **Save this** after upload; send on profile/post update. |
| **Presigned GET URL** | Short-lived display URL. **Do not** persist long-term or send back to the API as `media_urls`. |
| **External URL** | Legacy/third-party links (e.g. default cover). Use as-is; no signing. |

---

## Constraints

| Rule | Value |
|------|-------|
| Max file size | 10 MB per image |
| Allowed types | JPEG, PNG, GIF, WebP |
| Folders | `avatars`, `covers`, `posts` |
| Max images per post | 4 total (`media_keys` + `media_urls` combined) |
| Presigned GET TTL | **15 minutes** |
| Presigned PUT TTL | **10 minutes** |

---

## Upload flows

Choose one pattern per feature. **Pattern B** is recommended for mobile (less API bandwidth).

### Pattern A — Server-side multipart (simplest)

API receives the file and uploads to R2.

```http
POST {baseUrl}/api/v1/upload/avatar
Authorization: Bearer {accessToken}
Content-Type: multipart/form-data

file: <binary>
```

Endpoints:

| Use case | Endpoint |
|----------|----------|
| Avatar | `POST /api/v1/upload/avatar` |
| Cover | `POST /api/v1/upload/cover` |
| Post image | `POST /api/v1/upload/post` |

**Response:**

```json
{
  "message": "Avatar uploaded",
  "key": "avatars/user-123/550e8400-e29b-41d4-a716-446655440000.jpg",
  "url": "https://<account-id>.r2.cloudflarestorage.com/geoloc-media/avatars/...?X-Amz-Algorithm=..."
}
```

**Client must:**

- Keep `key` for the next attach step.
- May use `url` for immediate preview only (expires in 15 minutes).

**Flutter (Dio):**

```dart
Future<String> uploadAvatar(File imageFile) async {
  final formData = FormData.fromMap({
    'file': await MultipartFile.fromFile(
      imageFile.path,
      contentType: MediaType('image', 'jpeg'),
    ),
  });

  final response = await dio.post(
    '/api/v1/upload/avatar',
    data: formData,
  );

  final key = response.data['key'] as String;
  return key; // persist for profile update — not the url
}
```

---

### Pattern B — Direct upload to R2 (recommended for mobile)

Three steps: request presigned PUT → PUT file to R2 → attach `key`.

#### Step 1 — Get presigned PUT URL

```http
POST {baseUrl}/api/v1/media/upload-url
Authorization: Bearer {accessToken}
Content-Type: application/json

{
  "folder": "posts",
  "content_type": "image/jpeg",
  "filename": "photo.jpg"
}
```

**Response:**

```json
{
  "upload_url": "https://<account-id>.r2.cloudflarestorage.com/geoloc-media/posts/user-123/uuid.jpg?X-Amz-Algorithm=...",
  "key": "posts/user-123/550e8400-e29b-41d4-a716-446655440000.jpg",
  "expires_at": "2026-06-27T13:00:00Z"
}
```

#### Step 2 — PUT bytes to R2

```http
PUT {upload_url}
Content-Type: image/jpeg

<raw file bytes>
```

- **No** `Authorization` header on this request.
- `Content-Type` must **exactly** match the `content_type` from step 1.
- Complete within **10 minutes** of `expires_at`.

**Flutter:** use a plain `Dio` instance **without** the auth interceptor for the PUT to `upload_url`, or use `http.put`.

```dart
Future<void> putToR2(String uploadUrl, Uint8List bytes, String contentType) async {
  final client = Dio(); // no auth interceptor
  await client.put(
    uploadUrl,
    data: bytes,
    options: Options(
      headers: {'Content-Type': contentType},
      contentType: contentType,
    ),
  );
}
```

#### Step 3 — Attach `key` (see sections below)

---

## Attach uploaded media

Always send **keys** to mutating endpoints. Never send presigned URLs in `avatar_key`, `cover_key`, or `media_keys`.

### Profile — avatar & cover

```http
PUT {baseUrl}/api/v1/users/me
Authorization: Bearer {accessToken}
Content-Type: application/json

{
  "full_name": "John Smith",
  "avatar_key": "avatars/user-123/550e8400-e29b-41d4-a716-446655440000.jpg",
  "cover_key": "covers/user-123/550e8400-e29b-41d4-a716-446655440000.jpg"
}
```

- Omit fields you are not changing.
- Do **not** set `profile_picture_url` / `cover_image_url` to a presigned R2 URL — those fields are for external URLs only.
- Server replaces old owned R2 objects when you set a new `avatar_key` / `cover_key`.

### Create post — images

```http
POST {baseUrl}/api/v1/posts
Authorization: Bearer {accessToken}
Content-Type: application/json

{
  "content": "Beautiful sunset!",
  "media_keys": [
    "posts/user-123/550e8400-e29b-41d4-a716-446655440000.jpg",
    "posts/user-123/660e8400-e29b-41d4-a716-446655440001.png"
  ],
  "latitude": -6.3653,
  "longitude": 106.8269
}
```

- Up to **4** items across `media_keys` and `media_urls`.
- Prefer `media_keys` for Geoloc uploads.
- `media_urls` is for external links only (not presigned R2 URLs).

**Suggested post composer flow:**

1. User picks up to 4 images.
2. Upload each → collect `key`s (show local file or presigned preview while uploading).
3. On publish, `POST /posts` with `media_keys` only.
4. Render response `post.media_urls` (presigned GET URLs from server).

---

## Display media

### Where URLs appear

API responses resolve keys to presigned GET URLs in:

| Field | Endpoints |
|-------|-----------|
| `profile_picture_url` | Feed, posts, users, comments, search |
| `cover_image_url` | User profile |
| `media_urls` | Feed, posts, search |

Example value:

```
https://<account-id>.r2.cloudflarestorage.com/geoloc-media/posts/user-123/uuid.jpg?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Expires=900&...
```

### Critical display rules

1. **Do not attach JWT** to image requests. Presigned URLs authenticate via query parameters. Adding `Authorization` does not help and can break some HTTP stacks.
2. **Do not** configure your image widget to use the API auth interceptor for `media_urls` / avatars.
3. **Treat URLs as ephemeral.** They expire after ~15 minutes.
4. **External URLs** (no `r2.cloudflarestorage.com`, not your bucket) — load normally; they do not expire via signing.

**Flutter (`cached_network_image`):**

```dart
Widget postImage(String url) {
  return CachedNetworkImage(
    imageUrl: url,
    // Do NOT pass httpHeaders: {'Authorization': 'Bearer ...'}
    placeholder: (_, __) => const ShimmerBox(),
    errorWidget: (_, __, ___) => const Icon(Icons.broken_image),
  );
}
```

---

## Handling presigned URL expiry

### Symptoms

- Image loaded fine, then shows broken after app resume or ~15+ minutes.
- HTTP **403** from R2 on image GET.

### What not to do

| Anti-pattern | Why |
|--------------|-----|
| Persist presigned URLs in Hive/SQLite for offline feed | URLs stale on next launch |
| Cache images keyed only by URL in a long-TTL disk cache without refresh | Same issue |
| Store presigned URL in `media_urls` when creating a post | Server expects keys or external URLs |

### Recommended strategies

**Strategy 1 — Refetch parent resource (simplest)**

On image load error (403), refetch the post / profile / feed slice and replace URLs in local state.

**Strategy 2 — Refresh single URL via sign endpoint**

If you still have the object **key** (e.g. you stored it locally during compose, or parse from an old proxy URL — prefer keeping keys in your own model during upload):

```http
GET {baseUrl}/api/v1/media/sign?key=posts/user-123/uuid.jpg
Authorization: Bearer {accessToken}
```

```json
{
  "url": "https://<account-id>.r2.cloudflarestorage.com/...",
  "expires_at": "2026-06-27T13:15:00Z"
}
```

Then retry the image load with the new `url`.

**Strategy 3 — In-memory URL cache with expiry (best UX)**

```dart
class MediaUrlCache {
  final _entries = <String, _CachedUrl>{};

  String? get(String key) {
    final e = _entries[key];
    if (e == null || e.expiresAt.isBefore(DateTime.now())) return null;
    return e.url;
  }

  void put(String key, String url, DateTime expiresAt) {
    _entries[key] = _CachedUrl(url, expiresAt.subtract(const Duration(minutes: 1)));
  }
}
```

- Cache presigned URLs in memory only, with expiry slightly before server `expires_at`.
- On miss, call `/media/sign` or refetch the parent API resource.
- For feed scrolling, refetching the whole feed is usually enough; per-key sign is better for profile avatars shown across many screens.

### Offline feed cache

If you cache posts offline:

- Store post **content and metadata**, not presigned URLs, **or**
- Store URLs with `expires_at` and refresh on app foreground / pull-to-refresh.

---

## Delete media (optional)

Users can delete their own objects:

```http
DELETE {baseUrl}/api/v1/media/object?key=posts/user-123/uuid.jpg
Authorization: Bearer {accessToken}
```

Only succeeds when the key’s embedded user ID matches the authenticated user. Profile update with a new `avatar_key` already deletes the previous owned object server-side.

---

## End-to-end flows

### Avatar change

```
1. User picks image
2. POST /upload/avatar  (or Pattern B upload-url + PUT)
3. Receive key
4. PUT /users/me { "avatar_key": "<key>" }
5. Response user.profile_picture_url → presigned GET → show in UI
```

### Post with 2 images

```
1. User picks 2 images
2. Upload both → keys [k1, k2]
3. POST /posts { content, media_keys: [k1, k2], lat, lng }
4. Response post.media_urls → [presigned1, presigned2] → render carousel
```

### Feed scroll

```
1. GET /feed → posts with presigned media_urls and profile_picture_url
2. Render with CachedNetworkImage (no auth headers)
3. On 403 / broken image → refetch feed or sign keys
4. On pull-to-refresh → always get fresh presigned URLs
```

---

## Checklist for client implementation

- [ ] Upload returns `key`; attach flows use `avatar_key`, `cover_key`, or `media_keys`.
- [ ] Image widgets load presigned URLs **without** Bearer token.
- [ ] Post composer does not send presigned URLs in `media_urls`.
- [ ] Presigned PUT uses a separate HTTP client (no API auth on R2 PUT).
- [ ] `Content-Type` on R2 PUT matches `content_type` from upload-url request.
- [ ] Offline / long-lived cache does not rely on presigned URLs without refresh.
- [ ] Image error handler refetches or calls `/media/sign`.
- [ ] Max 4 images per post; max 10 MB per file; JPEG/PNG/GIF/WebP only.

---

## Common mistakes

| Mistake | Fix |
|---------|-----|
| Saving `url` from upload response and sending it in `media_urls` on post create | Send `key` in `media_keys` |
| Auth header on `CachedNetworkImage` | Remove headers for R2 URLs |
| Using API base URL + `/media/file?key=` | Use presigned URL from API response (legacy proxy is deprecated) |
| Caching feed overnight with image URLs | Refetch on launch or cache keys + refresh URLs |
| Reusing one Dio instance with auth interceptor for R2 PUT | Use plain client for `upload_url` host |

---

## Related docs

- [Media & Upload API](../api/media.md) — full server contract
- [Posts API](../api/posts.md) — create post payload
- [Users API](../api/users.md) — profile update
- [Feed API](../api/feed.md) — `media_urls` in feed items
- [Flutter Client Guide](./flutter.md) — project setup and packages
- [API Integration](./api-integration.md) — auth, pagination, caching patterns
