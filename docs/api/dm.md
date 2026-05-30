# Direct messages (DM) API

End-to-end encrypted direct messaging: the API stores **only ciphertext** (base64) and a **GCM nonce** (base64). The server never receives private keys or plaintext message bodies.

**Architecture:** [Direct messages (E2EE)](../architecture/dm.md) — component layout, data flows, Cassandra design.

**Postman:** Import `tests/postman/Geoloc_API.postman_collection.json` and use the **💬 Direct Messages (E2EE)** folder (variables: `peerUserId`, `conversationId`, `messageId`, `dmCursor`).

## Endpoint summary

| Method | Path | Rate limited |
|--------|------|----------------|
| `PUT` | `/api/v1/dm/keys` | Yes (60/min) |
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

Real-time delivery shares **`GET /api/v1/notifications/stream`** (see [Notifications](./notifications.md#sse-real-time-stream)).

## Security model

| Stored on server | Not stored / never sent |
|------------------|-------------------------|
| X25519 **public** keys per `key_version` | Private keys |
| AES-GCM ciphertext + nonce | Plaintext |
| Conversation membership metadata | Decryption keys |

Clients:

1. Generate an **X25519** identity keypair locally.
2. `PUT /dm/keys` to register the **public** key and monotonic `key_version`.
3. Before sending, `GET /dm/keys/:userID` for the peer’s public key, run **ECDH**, derive an AES-256 key (e.g. **HKDF**), then **AES-256-GCM** encrypt.
4. Send `ciphertext` + `nonce` + `key_version` (the version of the **recipient’s** key used for that message).

The server validates structure only: base64, nonce length **12** bytes, ciphertext length **≥ 17** bytes (minimum ciphertext expansion for GCM). It does not decrypt.

## SSE real-time

`GET /api/v1/notifications/stream` subscribes to:

- `sse:user:{userID}` — existing notification fan-out
- `dm:{userID}` — DM events (`dm_new_message`, `dm_read_receipt`)

Presence for push fallbacks uses Redis key `sse:online:{userID}` (set while the SSE connection is active).

## Kafka

When `KAFKA_BROKERS` is configured, the API opens a producer on topic **`dm_messages`**.

- After each successful send, a JSON event is published (partition key = recipient user id) if **`KAFKA_NOTIFICATIONS_ENABLED=true`** *or* the recipient has **no** `sse:online:{userID}` key (offline FCM pipeline hook).
- Read receipts are published to Kafka **only** when `KAFKA_NOTIFICATIONS_ENABLED=true`.

Payload fields align with SSE payloads; new-message events include `"type":"dm_new_message"` and `"event":"dm.message.created"`.

### SSE event examples

**New message** (`data:` line on the shared notification stream):

```json
{
  "type": "dm_new_message",
  "event": "dm.message.created",
  "conversation_id": "...",
  "message_id": "...",
  "sender_id": "...",
  "ciphertext": "<base64>",
  "nonce": "<base64>",
  "key_version": 1,
  "sender_key_version": 2,
  "sent_at": "2026-05-27T12:00:00.000000000Z"
}
```

**Read receipt:**

```json
{
  "type": "dm_read_receipt",
  "conversation_id": "...",
  "last_read_id": "...",
  "read_at": "2026-05-27T12:00:00.000000000Z"
}
```

Clients should branch on `type` (or `event` for Kafka consumers). Notification objects from `sse:user:{id}` do not include `type: dm_*`.

## Typical client flow

1. **Register key** — `PUT /api/v1/dm/keys` with your X25519 public key and `key_version`.
2. **Open SSE** — `GET /api/v1/notifications/stream` (same connection as in-app notifications).
3. **Start chat** — `POST /api/v1/dm/conversations` with `{ "user_id": "<peer>" }`; store `conversation_id`.
4. **Before first send** — `GET /api/v1/dm/keys/:peerUserId`; ECDH + HKDF → AES-256-GCM key; encrypt locally.
5. **Send** — `POST /api/v1/dm/conversations/:id/messages` with `ciphertext`, `nonce`, and the peer’s current `key_version`.
6. **History / catch-up** — `GET .../messages?cursor=` when reconnecting.
7. **Read state** — `PUT .../read` with `last_read_id` from the newest message you have decrypted.

If send returns **409** `key_version_mismatch`, re-fetch the peer’s key and retry.

## Multi-device and message history

**New device:** After generating keys, upload `PUT /dm/keys` with a new `key_version`. Restore your identity from `GET /dm/keys/backup` (passphrase-decrypted on the client) if you uploaded a backup with `PUT /dm/keys/backup`.

**Decrypting history:** Each message includes:

- `key_version` — recipient’s public key version used when the sender encrypted (for inbound messages to you).
- `sender_key_version` — sender’s public key version at send time (fetch via `GET /dm/keys/:senderID?key_version=N` or list versions with `GET /dm/keys/:userID/versions`).

For messages you sent, use your local plaintext or the backup; ciphertext on the server is encrypted for the **recipient**.

**Identity backup (opaque to server):** Client wraps the identity private key with a passphrase (e.g. PBKDF2 + AES-GCM). Upload `ciphertext`, `nonce`, and `kdf_salt` (base64, salt ≥ 16 decoded bytes). Only the authenticated user can read their backup.

## Error reference

| HTTP | `error` | Meaning |
|------|---------|---------|
| 400 | `invalid_body` | Missing or malformed JSON |
| 400 | `invalid_uuid` | Bad path or query UUID |
| 400 | `invalid_cursor` | Bad pagination cursor |
| 400 | `invalid_ciphertext_base64` | Ciphertext not valid base64 |
| 400 | `invalid_nonce_base64` | Nonce not valid base64 |
| 400 | `invalid_nonce_length` | Decoded nonce ≠ 12 bytes |
| 400 | `invalid_ciphertext_length` | Decoded ciphertext &lt; 17 bytes |
| 403 | `forbidden` | Not a conversation participant |
| 403 | `blocked` | Block relationship with peer |
| 404 | `public_key_not_found` | Peer has not uploaded a key |
| 404 | `backup_not_found` | No identity backup for caller |
| 409 | `key_version_mismatch` | `current_version` in body — refresh peer key |
| 409 | `recipient_has_no_public_key` | Recipient key missing at send time |
| 409 | `sender_has_no_public_key` | Caller must upload a key before sending |

## Cassandra migration

Apply `migrations/007_dm.cql` and `migrations/008_dm_multidevice.cql` to the `geoloc` keyspace.

---

## Endpoints

All routes require `Authorization: Bearer <access_token>`.

Write routes use per-user rate limiting: **60 requests/minute** (Redis), in addition to global IP limits.

### `PUT /api/v1/dm/keys`

Upload or rotate the caller’s public key.

**Request**

```json
{ "public_key": "<base64>", "key_version": 1 }
```

**Responses**

- `204 No Content` — success (re-uploading the same `(key_version, public_key)` is a no-op).
- `400` — `{"error":"invalid_body"}` / `invalid_key_version`

---

### `GET /api/v1/dm/keys/:userID`

Fetch another user’s public key. Without `key_version`, returns the **current** key (highest `key_version`). With `?key_version=N`, returns that version or `404`.

**Response `200`**

```json
{
  "user_id": "...",
  "public_key": "<base64>",
  "key_version": 1,
  "created_at": "2026-05-27T12:00:00.000000000Z"
}
```

**Errors**

- `404` — `{"error":"public_key_not_found"}`

---

### `GET /api/v1/dm/keys/:userID/versions`

List all public key versions for a user (newest clustering order).

**Response `200`**

```json
{
  "versions": [
    {
      "user_id": "...",
      "public_key": "<base64>",
      "key_version": 2,
      "created_at": "..."
    }
  ]
}
```

---

### `PUT /api/v1/dm/keys/backup`

Upload a passphrase-encrypted identity backup (caller only).

**Request**

```json
{
  "backup_version": 1,
  "ciphertext": "<base64>",
  "nonce": "<base64>",
  "kdf_salt": "<base64>"
}
```

**Responses**

- `204 No Content`
- `400` — invalid body / backup version / ciphertext / nonce / kdf_salt

---

### `GET /api/v1/dm/keys/backup`

Download your identity backup. Optional `?backup_version=N`; omit for latest.

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

**Errors**

- `404` — `{"error":"backup_not_found"}`

---

### `POST /api/v1/dm/conversations`

Create or return the canonical 1:1 conversation with another user.

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
    "created_at": "...",
    "last_message_at": "..."
  }
}
```

**Errors**

- `403` — `{"error":"blocked"}` if either user has blocked the other.
- `400` — invalid body / self-peer.

---

### `GET /api/v1/dm/conversations`

Paginated inbox (cursor = Cassandra page state).

**Query**

- `limit` — default `20`, max `100`
- `cursor` — opaque base64 page state from prior `next_cursor`

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
  "next_cursor": "<base64 or empty>"
}
```

---

### `DELETE /api/v1/dm/conversations/:id`

Remove the conversation from **your inbox only** (per-user delete). Does not delete messages or remove the chat for the other participant. Does not delete the canonical `dm_conversations` row.

**Responses**

- `204 No Content` — removed from inbox (idempotent if already hidden).
- `403` — `{ "error": "forbidden" }` if not a participant.

**Behavior**

- `GET /api/v1/dm/conversations` no longer lists this chat for you.
- `POST /api/v1/dm/conversations` with the same peer restores your inbox row so you can open the thread again.
- A new message in either direction re-adds both users’ inbox rows (existing send flow).

---

### `GET /api/v1/dm/conversations/:id/messages`

Message history (newest first). Non-participants receive **`403`** with `{"error":"forbidden"}` (not `404`).

**Query**

- `limit` — default `50`, max `100`
- `cursor` — base64 page state

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
      "sent_at": "...",
      "deleted_at": null
    }
  ],
  "next_cursor": "..."
}
```

`deleted_at` is an RFC3339 timestamp when the sender soft-deleted the message.

---

### `POST /api/v1/dm/conversations/:id/messages`

Send ciphertext. `key_version` must equal the **recipient’s** current server-side version.

**Request**

```json
{
  "ciphertext": "<base64>",
  "nonce": "<base64>",
  "key_version": 1
}
```

**Responses**

- `201 Created` — `{"message_id":"...","sent_at":"..."}`
- `409 Conflict` — `{"error":"key_version_mismatch","current_version":N}` (client should re-fetch keys and retry).
- `409 Conflict` — `{"error":"recipient_has_no_public_key"}` if the peer never uploaded a key.
- `409 Conflict` — `{"error":"sender_has_no_public_key"}` if the caller has no uploaded key.
- `403` — `{"error":"blocked"}` or `{"error":"forbidden"}` (not a participant).
- `400` — invalid base64 / nonce length / ciphertext length.

---

### `DELETE /api/v1/dm/messages/:messageID`

Soft-delete **your own** message. Requires query **`conversation_id`** (partition key).

Example: `DELETE /api/v1/dm/messages/{messageID}?conversation_id={uuid}`

**Responses**

- `204` — deleted or already tombstoned.
- `403` — not a participant, not the sender, or message not visible to you.

---

### `PUT /api/v1/dm/conversations/:id/read`

Update read receipt for the caller.

**Request**

```json
{ "last_read_id": "<timeuuid>" }
```

**Response**

- `204 No Content`
- `403` — not a participant.

The other participant receives a Redis/SSE payload:

```json
{
  "type": "dm_read_receipt",
  "conversation_id": "...",
  "last_read_id": "...",
  "read_at": "..."
}
```

---

## Conversation id

The server derives a deterministic `conversation_id` as **UUIDv5-style** (RFC 4122 name-based SHA-1) from a fixed namespace UUID and the string `min(userA,userB).String()+","+max(userA,userB).String()`, so `GetOrCreate` is idempotent.
