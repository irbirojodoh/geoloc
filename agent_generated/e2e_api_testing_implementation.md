# Security & Compliance Fixes — Walkthrough

## Summary

Resolved all critical and high-priority security issues from the beta readiness report across **16 files**.

## Changes Made

### 🔴 P0 Critical Fixes

| Fix | File(s) | What Changed |
|---|---|---|
| **Bcrypt password hashing** | [password.go](file:///Users/rijal/Documents/Projects/geoloc/internal/auth/password.go) | SHA3-512→SHA-256 chain replaced with bcrypt (cost 12). `HashPassword` now returns `(string, error)`. Timing-safe comparison via `bcrypt.CompareHashAndPassword`. |
| **JWT secret enforcement** | [jwt.go](file:///Users/rijal/Documents/Projects/geoloc/internal/auth/jwt.go) | Removed fallback default `"your-super-secret-key..."`. Server now `os.Exit(1)` if `JWT_SECRET` is unset. |
| **CORS restriction** | [main.go](file:///Users/rijal/Documents/Projects/geoloc/cmd/api/main.go) | `AllowOrigins: ["*"]` → reads from `ALLOWED_ORIGINS` env var (comma-separated), default `http://localhost:3000`. |
| **Error detail sanitization** | 13 handler files | Removed **all** `"details": err.Error()` from user-facing JSON. Errors logged server-side via `slog.Error()` with structured context. |

### 🟡 P1 High Fixes

| Fix | File(s) | What Changed |
|---|---|---|
| **Login brute-force protection** | [auth.go](file:///Users/rijal/Documents/Projects/geoloc/internal/handlers/auth.go) | In-memory rate limiter: 5 failed attempts per identifier within 15 minutes triggers lockout. Returns `429 Too Many Requests`. Clears on successful login. |
| **Upload magic byte validation** | [upload.go](file:///Users/rijal/Documents/Projects/geoloc/internal/handlers/upload.go) | File content type now detected via `http.DetectContentType()` (reads first 512 bytes) instead of trusting the `Content-Type` header. |
| **Input length limits** | [models.go](file:///Users/rijal/Documents/Projects/geoloc/internal/data/models.go), [auth.go](file:///Users/rijal/Documents/Projects/geoloc/internal/handlers/auth.go) | Post content: max 5000 chars. Bio: max 500. Full name: max 100. Password: max 128. |
| **ALLOWED_ORIGINS env var** | [.env.example](file:///Users/rijal/Documents/Projects/geoloc/.env.example) | Documented new `ALLOWED_ORIGINS` configuration. |

### ⏳ Deferred

- **CSRF protection** — Deferred as it requires frontend coordination (token injection into forms/headers).

## Files Modified

```
internal/auth/password.go       — bcrypt hashing
internal/auth/jwt.go            — no default secret
internal/auth/auth_test.go      — test updated for (string, error) return
internal/handlers/auth.go       — brute-force + error sanitization
internal/handlers/post.go       — error sanitization
internal/handlers/upload.go     — magic byte validation + error sanitization
internal/handlers/comment.go    — error sanitization
internal/handlers/like.go       — error sanitization
internal/handlers/follow.go     — error sanitization
internal/handlers/user.go       — error sanitization
internal/handlers/notification.go — error sanitization
internal/handlers/search.go     — error sanitization
internal/handlers/location.go   — error sanitization
internal/handlers/geocode.go    — error sanitization + slog
internal/handlers/device.go     — error sanitization
internal/handlers/profile.go    — error sanitization
internal/handlers/oauth.go      — error sanitization
internal/data/models.go         — field length limits
cmd/api/main.go                 — CORS config
.env.example                    — ALLOWED_ORIGINS
```

## Verification

- ✅ `go build ./...` — zero errors
- ✅ All 6 auth tests pass (bcrypt hashing, JWT lifecycle, middleware)
- ✅ Zero `"details": err.Error()` in any handler (grep verified)

> [!NOTE]
> Existing passwords hashed with the old SHA scheme will no longer validate. Since the app isn't live, re-seed data with `make docker-up` to regenerate test users with bcrypt hashes.

---

# E2E API Testing Implementation

## Summary
Successfully established a fully functional End-to-End (E2E) testing suite governing all endpoints in the application.

## Changes Made
-  **E2E Test File (`e2e_test.go`)**: Created a comprehensive test suite utilizing `testcontainers-go` to spin up Cassandra DB locally during testing. Consists of 35+ granular integration tests targeting every single application route handler.
- **Fixed `cover_image_url` Schema Mismatch**: Fixed a bug block in `internal/data/user_repo.go` where CQL queries were crashing because they expected the `cover_image_url` column which wasn't fully supported across environments.
- **Schema Updates**: Appended missing tables `like_state` and `likes_by_user` inside `cassandra_schema.cql` which was necessary for `testcontainers` testing to validate Like workflows correctly.

The test suite validates components including:
1. **Authentication:** Registration, Login (Username/Email), Invalid Credentials, non-existent Users.
2. **User Management:** Get My Profile, Get Profiles by ID/Username, Updating Profiles, Authorization checking.
3. **Posts:** Full CRUD emulation. Create, Retrieve, Feed, Validation.
4. **Comments:** Create Comments, View Post Comments, Reply mechanics, Validations.
5. **Likes:** Liking Posts, Unliking Posts, Toggle Logic. Liking/Unliking Comments.
6. **Follows:** Real-time following mechanics, Cannot-self-follow, Retrieve Followers.
7. **Search:** Querying users and posts using parameterized strings.
8. **Locations:** Location following mechanics.
9. **No Error Detail Leaking Verification:** Strict checks asserting handlers don't leak 500 error traces to users.
10. **OAuth Integration:** Stubbing and verifying payload handling.

## Verification
- ✅ `go test -v ./internal/handlers/...` successfully runs and passes all 35 tests, standing up and tearing down `cassandra` environments automatically.
- ✅ `go build ./...` confirmed zero compilation errors resulting from test infrastructure imports.
