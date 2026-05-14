# Push notification testing (FCM)

Guide for testing push notifications locally with Postman — without a native mobile app.

## Prerequisites

- API running with `KAFKA_NOTIFICATIONS_ENABLED=true`
- Migration `006_notifications_v2.cql` applied (`push_device_tokens` table)
- For **Level 1** (log-only): no Firebase setup
- For **Level 2** (real FCM): Firebase project + service account — see below

## Level 1 — Pipeline test (no Firebase)

Uses `LogPushService` when `PUSH_NOTIFICATIONS_ENABLED` is not `true`.

1. Register a fake device token:
   ```http
   POST /api/v1/devices
   Authorization: Bearer <accessToken>

   { "token": "fake-dev-token", "platform": "web" }
   ```
2. As a **second user**, follow or toggle-like the recipient's post.
3. Check API logs for `[PUSH] Sending to device...`
4. Check in-app: `GET /api/v1/notifications`

## Level 2 — Real FCM (web token)

### 1. Firebase setup

1. Create a Firebase project and register a **Web** app.
2. Generate a **Web Push certificate** (VAPID key pair) under Cloud Messaging.
3. Download a **service account** JSON (Project settings → Service accounts).

### 2. Configure API

In `.env.development`:

```env
KAFKA_BROKERS=127.0.0.1:9092
KAFKA_NOTIFICATIONS_ENABLED=true
PUSH_NOTIFICATIONS_ENABLED=true
FCM_PROJECT_ID=your-project-id
FCM_CREDENTIALS_JSON={"type":"service_account",...}
```

Restart API — expect `FCM service initialized` in logs.

### 3. Get a web FCM token

```bash
cd tests/fcm_client_test
python3 -m http.server 3000
```

1. Set `vapidKey` in `firebase-config.js` (VAPID **public** key from Firebase Console).
2. Open `http://localhost:3000/fcm-test.html` (use `localhost`, not `127.0.0.1`).
3. Click **Get FCM token** — copy the long string shown (not the VAPID key).

### 4. Register device (recipient user)

```http
POST /api/v1/devices
Authorization: Bearer <recipientAccessToken>

{
  "token": "<FCM web token>",
  "platform": "web"
}
```

Expect `200` with `"message": "Device registered"`.

Verify in Cassandra:

```bash
docker exec geoloc-cassandra-1 cqlsh -e \
  "SELECT user_id, platform, fcm_token FROM geoloc.push_device_tokens LIMIT 5;"
```

### 5. Trigger notification (second user)

Use a **fresh JWT** (access tokens expire after 15 minutes).

| Action | Endpoint |
|--------|----------|
| Follow | `POST /api/v1/users/:id/follow` |
| Like | `POST /api/v1/posts/:id/toggle-like` (no body = true toggle) |

**Important:** Like notifications are only sent from **`toggle-like`**, not legacy `POST .../like`.

### 6. Verify delivery

| Check | How |
|-------|-----|
| In-app | `GET /api/v1/notifications` as recipient |
| API logs | No FCM errors in `notif-push-dispatch` |
| Browser | Keep `fcm-test.html` tab open for foreground alerts |
| Firebase Console | Messaging → Send test message → paste FCM token |

## Two users in Postman

Use two collection variables (`accessTokenA`, `accessTokenB`) or two environments. Re-login when you see `"Token has expired"` (15 min TTL).

## Apply migration manually

```bash
docker cp migrations/006_notifications_v2.cql geoloc-cassandra-1:/tmp/
docker exec geoloc-cassandra-1 cqlsh -f /tmp/006_notifications_v2.cql
```

## Troubleshooting

| Issue | Fix |
|-------|-----|
| `500` on `POST /api/v1/devices` | Run migration `006`; restart API after `device_query.go` schema fix |
| `Token has expired` | Re-login or `POST /auth/refresh` |
| Like notification missing | Use `toggle-like`, not `POST .../like` |
| `changed: false` on every toggle | Fixed: empty body now flips state (restart API) |
| FCM SW registration error | Serve from `tests/fcm_client_test/`; use `localhost:3000` |
