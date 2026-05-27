# Direct messages (DM) API

End-to-end encrypted direct messaging: the API stores **only ciphertext** (base64) and a **GCM nonce** (base64). The server never receives private keys or plaintext message bodies.

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

## Cassandra migration

Apply `migrations/007_dm.cql` to the `geoloc` keyspace (see file for full DDL).

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

Fetch another user’s **current** public key (highest `key_version`).

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
