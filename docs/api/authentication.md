# Authentication

Geoloc uses JWT (JSON Web Tokens) for authentication.

## Token Types

| Token | Lifetime | Purpose |
|-------|----------|---------|
| Access Token | 15 minutes | API requests |
| Refresh Token | 7 days | Get new access tokens |

## Register

Create a new user account.

**Endpoint:** `POST /auth/register`

**Request:**
```json
{
  "username": "john_doe",
  "email": "john@example.com",
  "password": "securepassword123",
  "full_name": "John Doe"
}
```

**Response:** `201 Created`
```json
{
  "message": "User registered successfully",
  "user_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

## Login

Authenticate and receive tokens.

**Endpoint:** `POST /auth/login`

**Request:**
```json
{
  "identifier": "john_doe",
  "password": "securepassword123"
}
```

> **Note:** `identifier` can be username or email.

**Response:** `200 OK`
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_in": 900,
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "username": "john_doe",
    "email": "john@example.com",
    "full_name": "John Doe"
  }
}
```

## Refresh Token

Get a new access token when the current one expires.

**Endpoint:** `POST /auth/refresh`

**Request:**
```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
}
```

**Response:** `200 OK`
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_in": 900
}
```

## Using Access Token

Include the access token in the `Authorization` header:

```
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

**Example:**
```bash
curl http://localhost:8080/api/v1/feed?latitude=-6.36&longitude=106.82 \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..."
```

## Token Refresh Flow

```
┌─────────────────────────────────────────────────────────────┐
│                     Client Application                       │
└─────────────────────────────────────────────────────────────┘
                             │
                             ▼
           ┌─────────────────────────────────┐
           │     Is access token expired?    │
           └─────────────────────────────────┘
                    │              │
                   Yes            No
                    │              │
                    ▼              ▼
        ┌───────────────────┐    Use token for
        │ POST /auth/refresh │    API request
        └───────────────────┘
                    │
                    ▼
           Get new access token
```

## Error Responses

| Status | Meaning |
|--------|---------|
| `400 Bad Request` | Invalid request body |
| `401 Unauthorized` | Invalid credentials or expired token |
| `404 Not Found` | User not found |
| `409 Conflict` | Username or email already exists |
