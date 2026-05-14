# FCM web token test client

Minimal browser page to obtain an FCM **registration token** for Level 2 push testing.

## Setup

1. Copy your Firebase web app config and VAPID public key into `firebase-config.js`.
2. Ensure `firebase-messaging-sw.js` uses the same Firebase config.

## Run

```bash
cd tests/fcm_client_test
python3 -m http.server 3000
```

Open `http://localhost:3000/fcm-test.html` and click **Get FCM token**.

Use the displayed string (not the VAPID key) in Postman `POST /api/v1/devices`.

See [docs/testing-push-notifications.md](../../docs/testing-push-notifications.md).
