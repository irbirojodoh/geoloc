# Geoloc Mobile — Direct Messages (E2EE) Implementation Guide

**Audience:** Mobile / Flutter coding agent implementing encrypted 1:1 DMs against the Geoloc backend.

**Prerequisites (assume already in the app):**

- HTTP client with JWT (`Authorization: Bearer <accessToken>`) and token refresh on `401`
- Secure storage for auth tokens
- Existing notifications list + SSE connection to `GET /api/v1/notifications/stream`
- State management (e.g. Riverpod/Bloc), routing, user profile fetch

**Base URL (dev):** `http://localhost:8080` — use your app’s configured API base.

---

## Objective

Implement **end-to-end encrypted direct messaging**:

- Server stores **ciphertext + nonce + metadata only** — never plaintext, never private keys.
- Client generates keys, encrypts/decrypts locally, syncs via REST + the **existing SSE** stream.
- UX: inbox, chat thread, send/receive, read receipts, soft-delete tombstones, block handling.

**Do not** send plaintext in API bodies or upload private keys.

---

## Backend context

| Layer | Detail |
|-------|--------|
| API | REST JSON, JWT on all `/api/v1/*` routes |
| DM routes | `/api/v1/dm/*` |
| Real-time | **Same SSE** as notifications — DM events arrive on the existing stream |
| Write rate limit | DM write endpoints: **60 requests/minute** per user |

Conversation IDs are **deterministic** for a user pair. `POST /dm/conversations` is idempotent — safe to call again for the same peer.

---

## Authentication

All DM endpoints require:

```http
Authorization: Bearer <accessToken>
```

Access tokens expire after **15 minutes**. On `401` with an expired-token message, refresh via your existing auth flow (`POST /auth/refresh` with refresh token) and retry.

**Standard error body** (all API errors):

```json
{ "error": "snake_case_reason" }
```

Some errors include extra fields, e.g. `409 key_version_mismatch`:

```json
{
  "error": "key_version_mismatch",
  "current_version": 2
}
```

---

## Critical security rules

1. **Private keys** → platform secure storage only (Keychain / Keystore). Never log, upload, or analytics-track.
2. **Plaintext** → memory/UI only after decrypt or while composing. Never in HTTP JSON.
3. **`key_version` on send** → must be the **recipient’s** current version from `GET /dm/keys/:userId`, not the sender’s.
4. **409 `key_version_mismatch`** → re-fetch recipient key, use `current_version`, re-encrypt, retry once.
5. **403 `blocked`** → show “You can’t message this user”; hide Message action when possible.
6. **403 `forbidden`** on conversation routes → user is not a participant (not 404).

---

## Cryptography (client-side)

Simplified X3DH-inspired model. **iOS and Android must use identical derivation** or messages will not decrypt. Full Signal Double Ratchet is **not** required for v1.

### Algorithms

| Step | Algorithm |
|------|-----------|
| Identity keys | **X25519** keypair per user |
| Shared secret | **X25519 ECDH** (your private × peer public) → 32 bytes |
| Message key | **HKDF-SHA256** → 32-byte AES key |
| Encrypt | **AES-256-GCM**, **12-byte random nonce** per message |
| Wire format | **Base64** for `public_key`, `ciphertext`, `nonce` in JSON |

### Canonical HKDF (MUST match on all clients)

```text
ikm  = X25519(privateKey_self, publicKey_peer)   // 32 bytes
salt = UTF-8("geoloc-dm")
info = UTF-8("dm-aes-key-v1")
aesKey = HKDF-SHA256(ikm, salt, info, length=32)
```

Base64-decode the peer’s `public_key` from the API. Cache `aesKey` in memory keyed by `(peerUserId, key_version)`.

### Encrypt (outgoing)

```text
1. plaintext = UTF-8 string
2. nonce = 12 random bytes (CSPRNG)
3. ciphertext = AES-256-GCM.encrypt(aesKey, nonce, plaintext)
4. POST:
   {
     "ciphertext": base64(ciphertext_including_tag),
     "nonce": base64(nonce),
     "key_version": <recipient's current key_version>
   }
```

Server validation: decoded nonce **exactly 12 bytes**; decoded ciphertext **≥ 17 bytes**.

### Decrypt (incoming)

```text
1. GET /dm/keys/:senderId if key for message.key_version not cached
2. aesKey = derive(yourPrivateKey, senderPublicKey, message.key_version)
3. plaintext = AES-256-GCM.decrypt(aesKey, base64Decode(nonce), base64Decode(ciphertext))
```

In 1:1 chat the sender is always the other participant.

### Key lifecycle

| Event | Action |
|-------|--------|
| First login, no local key | Generate X25519 pair → `PUT /dm/keys` with `key_version: 1` |
| New device | `GET /dm/keys/backup` → passphrase-decrypt identity → `PUT /dm/keys` with new version if rotating |
| User rotates key | Increment version → `PUT` new public key; keep old versions on server for history |
| Peer version changes | Invalidate cached aesKey; re-fetch on next send/receive |
| Decrypt old inbound msg | Use `message.sender_key_version` → `GET /dm/keys/:senderId?key_version=N` |
| 409 on send | `GET /dm/keys/:recipientId` → retry with `current_version` |

### Multi-device identity backup

Before or after first `PUT /dm/keys`, optionally upload a **passphrase-wrapped** backup of the identity private key:

```json
PUT /api/v1/dm/keys/backup
{
  "backup_version": 1,
  "ciphertext": "<base64 AES-GCM blob>",
  "nonce": "<base64 12 bytes>",
  "kdf_salt": "<base64 ≥16 bytes>"
}
```

The server stores this opaque blob. On a new device: `GET /api/v1/dm/keys/backup`, derive key from user passphrase + `kdf_salt`, decrypt locally, restore identity, then register public key.

**Message history:** Each message includes `key_version` (recipient key used) and `sender_key_version` (sender’s key at encrypt time). List versions with `GET /dm/keys/:userId/versions` or fetch one with `?key_version=N`. Messages you sent are encrypted for the recipient — keep local plaintext or rely on backup; server ciphertext alone cannot decrypt your own sends.

### Dart packages (suggestion)

```yaml
dependencies:
  cryptography: ^2.7.0   # X25519, HKDF, AES-GCM
  flutter_secure_storage: ^9.0.0
```

Implement one **`DmCryptoService`** (pure Dart, unit-tested). No crypto in widgets.

---

## REST API — complete contract

### Summary

| Method | Path | Rate limited |
|--------|------|--------------|
| `PUT` | `/api/v1/dm/keys` | Yes |
| `PUT` | `/api/v1/dm/keys/backup` | Yes |
| `GET` | `/api/v1/dm/keys/backup` | No |
| `GET` | `/api/v1/dm/keys/:userID` | No |
| `GET` | `/api/v1/dm/keys/:userID/versions` | No |
| `POST` | `/api/v1/dm/conversations` | Yes |
| `GET` | `/api/v1/dm/conversations` | No |
| `GET` | `/api/v1/dm/conversations/:id/messages` | No |
| `POST` | `/api/v1/dm/conversations/:id/messages` | Yes |
| `DELETE` | `/api/v1/dm/messages/:messageID?conversation_id=` | Yes |
| `PUT` | `/api/v1/dm/conversations/:id/read` | Yes |
| `DELETE` | `/api/v1/dm/conversations/:id` | Yes |

---

### `PUT /api/v1/dm/keys`

Upload or rotate the caller’s public key.

**Request**

```json
{
  "public_key": "<base64-encoded X25519 public key>",
  "key_version": 1
}
```

**Responses**

- `204 No Content` — success; re-uploading same `(key_version, public_key)` is a no-op
- `400` — `{ "error": "invalid_body" }` or `{ "error": "invalid_key_version" }`

---

### `GET /api/v1/dm/keys/:userID`

Fetch another user’s public key. Without query params, returns **current** (highest `key_version`). With `?key_version=N`, returns that version.

**Response `200`**

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "public_key": "cHVibGljLWtleS1kYXRh",
  "key_version": 1,
  "created_at": "2026-05-27T12:00:00.000000000Z"
}
```

**Errors**

- `404` — `{ "error": "public_key_not_found" }`

---

### `GET /api/v1/dm/keys/:userID/versions`

List all public key versions (newest first).

**Response `200`**

```json
{
  "versions": [
    {
      "user_id": "...",
      "public_key": "...",
      "key_version": 2,
      "created_at": "..."
    }
  ]
}
```

---

### `PUT /api/v1/dm/keys/backup`

Upload passphrase-encrypted identity backup (caller only). Same ciphertext/nonce validation as messages; `kdf_salt` must decode to ≥16 bytes.

**Request**

```json
{
  "backup_version": 1,
  "ciphertext": "...",
  "nonce": "...",
  "kdf_salt": "..."
}
```

**Responses:** `204` or `400` (`invalid_backup_version`, `invalid_kdf_salt_base64`, etc.)

---

### `GET /api/v1/dm/keys/backup`

Download your backup. Optional `?backup_version=N`; omit for latest.

**Response `200`**

```json
{
  "backup_version": 1,
  "ciphertext": "...",
  "nonce": "...",
  "kdf_salt": "...",
  "updated_at": "..."
}
```

**Errors:** `404` — `{ "error": "backup_not_found" }`

---

### `POST /api/v1/dm/conversations`

Start or retrieve the canonical 1:1 conversation.

**Request**

```json
{ "user_id": "<peer uuid>" }
```

**Response `200`**

```json
{
  "conversation": {
    "conversation_id": "...",
    "participant_a": "...",
    "participant_b": "...",
    "created_at": "2026-05-27T12:00:00.000000000Z",
    "last_message_at": "2026-05-27T12:00:00.000000000Z"
  }
}
```

**Errors**

- `403` — `{ "error": "blocked" }`
- `400` — invalid body / self-peer (`invalid_peer`)

---

### `GET /api/v1/dm/conversations`

Inbox list, newest first.

**Query**

| Param | Default | Max | Description |
|-------|---------|-----|-------------|
| `limit` | 20 | 100 | Page size |
| `cursor` | — | — | Opaque base64 from previous `next_cursor` |

**Response `200`**

```json
{
  "conversations": [
    {
      "conversation_id": "...",
      "other_user_id": "...",
      "last_message_at": "...",
      "created_at": "..."
    }
  ],
  "next_cursor": "<base64 or empty string>"
}
```

Empty `next_cursor` means no more pages.

---

### `DELETE /api/v1/dm/conversations/:id`

Remove the conversation from **your inbox only**. Messages stay on the server; the peer’s inbox is unchanged.

**Response:** `204 No Content`

**Errors:** `403` — `{ "error": "forbidden" }`

**Behavior**

- Hidden from your inbox list until you `POST /dm/conversations` with the same peer again, or a new message arrives (send flow re-adds inbox rows for both participants).

---

### `GET /api/v1/dm/conversations/:id/messages`

Message history, **newest first**.

**Query:** same `limit` (default 50, max 100) and `cursor` as inbox.

**Response `200`**

```json
{
  "messages": [
    {
      "message_id": "...",
      "sender_id": "...",
      "ciphertext": "...",
      "nonce": "...",
      "key_version": 1,
      "sender_key_version": 2,
      "sent_at": "2026-05-27T12:00:00.000000000Z",
      "deleted_at": null
    }
  ],
  "next_cursor": "..."
}
```

When soft-deleted, `deleted_at` is an RFC3339 timestamp — show tombstone UI (“Message deleted”), do not decrypt.

**Errors**

- `403` — `{ "error": "forbidden" }` (not a participant; do not map to 404)

---

### `POST /api/v1/dm/conversations/:id/messages`

Send encrypted message. **`key_version` must match recipient’s current server version.**

**Request**

```json
{
  "ciphertext": "<base64>",
  "nonce": "<base64>",
  "key_version": 1
}
```

**Response `201`**

```json
{
  "message_id": "...",
  "sent_at": "2026-05-27T12:00:00.000000000Z"
}
```

**Errors**

- `409` — `{ "error": "key_version_mismatch", "current_version": 2 }`
- `409` — `{ "error": "recipient_has_no_public_key" }`
- `403` — `{ "error": "blocked" }` or `{ "error": "forbidden" }`
- `400` — `invalid_ciphertext_base64`, `invalid_nonce_base64`, `invalid_nonce_length`, `invalid_ciphertext_length`

---

### `DELETE /api/v1/dm/messages/:messageID`

Soft-delete **your own** message only.

**Required query:** `conversation_id=<uuid>`

Example: `DELETE /api/v1/dm/messages/{messageID}?conversation_id={uuid}`

**Responses**

- `204` — deleted or already tombstoned
- `403` — not participant, not sender, or message not found for you

---

### `PUT /api/v1/dm/conversations/:id/read`

Update read receipt for the authenticated user.

**Request**

```json
{ "last_read_id": "<timeuuid of newest message you've read>" }
```

**Response:** `204 No Content`

**Errors:** `403` — `{ "error": "forbidden" }`

Peer receives real-time `dm_read_receipt` on SSE (see below).

---

## Related APIs (inbox UX)

These are **not** DM routes but needed for UI:

**User profile (avatar, name for inbox rows):**

```http
GET /api/v1/users/:id
Authorization: Bearer <token>
```

**Block check (optional client-side; server enforces on DM):**

```http
POST   /api/v1/users/:id/block
DELETE /api/v1/users/:id/block
GET    /api/v1/users/me/blocked
```

If either party blocked the other, DM returns `403` `{ "error": "blocked" }`.

---

## Real-time — SSE (extend existing connection)

**Do not open a second SSE connection.** Reuse:

```http
GET /api/v1/notifications/stream
Authorization: Bearer <accessToken>
Accept: text/event-stream
```

Long-lived stream. Heartbeat comment every ~30s (`: heartbeat`). Initial event:

```text
event: connected
data: {"status":"connected","time":"2026-05-27T12:00:00.000000000Z"}
```

Subsequent lines are `data: {json}\n\n`. Two payload families share this stream:

### A. In-app notifications (existing)

No `type` field starting with `dm_`. Typical shape:

```json
{
  "id": "notif-uuid",
  "user_id": "recipient-uuid",
  "type": "like",
  "actor_id": "actor-uuid",
  "target_type": "post",
  "target_id": "post-uuid",
  "message": "liked your post",
  "payload": {},
  "is_read": false,
  "created_at": "2026-05-27T12:00:00.000000000Z"
}
```

Route to your existing notification handler.

### B. DM events (new)

Branch when `json['type']` is `dm_new_message` or `dm_read_receipt`:

**New message**

```json
{
  "type": "dm_new_message",
  "event": "dm.message.created",
  "conversation_id": "...",
  "message_id": "...",
  "sender_id": "...",
  "ciphertext": "...",
  "nonce": "...",
  "key_version": 1,
  "sender_key_version": 2,
  "sent_at": "2026-05-27T12:00:00.000000000Z"
}
```

**Read receipt**

```json
{
  "type": "dm_read_receipt",
  "conversation_id": "...",
  "last_read_id": "...",
  "read_at": "2026-05-27T12:00:00.000000000Z"
}
```

**Parser rule:**

```dart
final type = json['type'] as String?;
if (type != null && type.startsWith('dm_')) {
  handleDmEvent(json);
} else {
  handleAppNotification(json);
}
```

**Delivery strategy**

- Foreground + chat open → decrypt SSE payload, append bubble
- Foreground + other screen → update inbox preview + unread count
- Background/killed → FCM wake (if configured) then `GET .../messages` on resume for gap fill
- Reconnect SSE with exponential backoff on disconnect (same as notifications)

While SSE is connected, the server marks the user online for push routing; offline users may get push via backend Kafka/FCM pipeline — client should still poll history on resume.

---

## Error reference (full)

| HTTP | `error` | Client action |
|------|---------|---------------|
| 400 | `invalid_body` | Fix request JSON |
| 400 | `invalid_uuid` | Fix path/query UUID |
| 400 | `invalid_cursor` | Reset pagination |
| 400 | `invalid_key_version` | Use positive integer |
| 400 | `invalid_ciphertext_base64` | Re-encode ciphertext |
| 400 | `invalid_nonce_base64` | Re-encode nonce |
| 400 | `invalid_nonce_length` | Nonce must decode to 12 bytes |
| 400 | `invalid_ciphertext_length` | Ciphertext too short |
| 403 | `forbidden` | Not a participant |
| 403 | `blocked` | Show blocked UX |
| 404 | `public_key_not_found` | Peer has no DM key yet |
| 404 | `backup_not_found` | No backup uploaded yet |
| 409 | `key_version_mismatch` | Use `current_version`, re-fetch key, retry |
| 409 | `recipient_has_no_public_key` | Peer must open app / upload key |
| 409 | `sender_has_no_public_key` | Upload your public key before sending |
| 429 | rate limit | Exponential backoff |

---

## Data models (Dart)

```dart
class DmPublicKey {
  final String userId;
  final String publicKeyBase64;
  final int keyVersion;
  final DateTime createdAt;
}

class DmConversation {
  final String conversationId;
  final String otherUserId;
  final DateTime lastMessageAt;
  final DateTime createdAt;
  final String? lastMessagePreview;  // local only
  final int unreadCount;             // local only
}

class DmMessage {
  final String messageId;
  final String conversationId;
  final String senderId;
  final String ciphertextBase64;
  final String nonceBase64;
  final int keyVersion;
  final int senderKeyVersion;
  final DateTime sentAt;
  final DateTime? deletedAt;
  final String? plaintext;           // local only, after decrypt
  final bool decryptFailed;          // local only
}

class DmReadReceipt {
  final String conversationId;
  final String lastReadId;
  final DateTime readAt;
}
```

Persist ciphertext locally; plaintext optional in local DB only — never sent to server.

---

## Suggested project layout

```text
lib/
├── core/crypto/dm_crypto_service.dart
├── data/
│   ├── models/dm_*.dart
│   ├── datasources/remote/dm_remote_datasource.dart
│   ├── datasources/local/dm_local_datasource.dart
│   └── repositories/dm_repository_impl.dart
├── domain/
│   ├── repositories/dm_repository.dart
│   └── usecases/
│       ├── ensure_dm_keys_uploaded.dart
│       ├── get_inbox.dart
│       ├── get_messages.dart
│       ├── send_dm_message.dart
│       └── mark_conversation_read.dart
└── presentation/
    ├── screens/messages/inbox_screen.dart
    ├── screens/messages/chat_screen.dart
    └── providers/dm_providers.dart
```

**`DmRepository`:** key upload after login, encrypt/decrypt, REST calls, SSE event dispatch to UI state.

---

## UI requirements

### Entry points

- Messages tab / icon → inbox
- User profile → **Message** → `POST /dm/conversations` → chat screen

### Inbox

- `GET /dm/conversations` with pull-to-refresh + infinite scroll (`next_cursor`)
- Row: peer avatar/name (`GET /users/:id`), preview, time, unread badge
- Tap → chat

### Chat

- Newest at bottom; load older via `cursor`
- Sent/received bubbles; tombstone if `deleted_at != null`
- Composer + send; disable if peer has no public key
- Long-press own message → soft delete
- Debounced `PUT .../read` when viewing messages
- Read ticks on `dm_read_receipt` SSE

### Key setup

After login (background): generate/load keys → `PUT /dm/keys`. Banner if unavailable.

### Blocks

Respect block list client-side; handle `403 blocked` from server.

---

## Lifecycle flows

**After login**

```text
ensureDmKeysUploaded() → PUT /dm/keys if needed
```

**Open chat with user B**

```text
POST /dm/conversations { user_id: B }
GET /dm/keys/B
GET /dm/conversations/{id}/messages?limit=50 → decrypt → render
PUT /dm/conversations/{id}/read (debounced)
```

**Send**

```text
GET /dm/keys/B (if stale)
encrypt → POST .../messages
409 → refresh key with current_version → retry once
201 → append to UI with message_id
```

**SSE receive**

```text
dm_new_message → decrypt → update thread or inbox
dm_read_receipt → update read UI
```

**Logout**

Clear secure DM keys and local message cache.

---

## Local storage

| Store | Key / table | Content |
|-------|-------------|---------|
| Secure | `dm_private_key` | X25519 private bytes |
| Secure | `dm_public_key_version` | int |
| Local DB | conversations | metadata + preview |
| Local DB | messages | ciphertext fields + optional plaintext |

---

## Integration checklist

- [ ] `DmCryptoService` + unit tests (round-trip, fixed HKDF vectors)
- [ ] All 8 DM REST endpoints
- [ ] SSE branch for `dm_*` types on existing stream
- [ ] `ensureDmKeysUploaded()` after auth
- [ ] Inbox + chat screens + profile Message action
- [ ] 409 retry with `current_version`
- [ ] Cursor pagination (inbox + messages)
- [ ] Read receipts + soft delete
- [ ] Logout clears DM secrets

---

## Testing

**Unit:** HKDF/AES-GCM round-trip; nonce length 12; ciphertext ≥ 17 bytes encoded.

**Manual (two test accounts):**

1. Both register/login; keys upload automatically
2. A messages B — “Hello” decrypts on B’s device
3. B replies — A receives
4. Bump B’s `key_version` — A’s next send still works (409 path or pre-fetch)
5. Block → send fails with `blocked`
6. Delete → tombstone in history
7. SSE delivers message while app foreground

**Never** send human-readable plaintext in the send-message API body — only base64 ciphertext.

---

## Out of scope (v1)

- Group chats
- Attachments / media in DMs
- Full Double Ratchet / per-message forward secrecy
- Edit message
- Typing indicators
- Server-side message search

---

## Definition of done

- Inbox + chat + profile Message work end-to-end
- Network payloads contain only base64 ciphertext (verify with a proxy)
- SSE delivers DMs while app is open; history sync on resume
- Read receipts and delete work
- Key version mismatch self-heals
- No private keys or plaintext in logs or API bodies
