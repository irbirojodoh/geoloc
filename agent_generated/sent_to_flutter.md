# Geoloc Flutter App - Backend Updates & Refactoring Guide

## Objective
The Geoloc backend (Go + Cassandra) has just undergone a major architectural and security hardening overhaul. 

Since you already have the initial Flutter codebase, **your task is to refactor your existing implementation** to align with these new backend security rules and endpoint changes.

---

## 🚨 1. Authentication Overhaul: Mobile-Native OAuth

**The Old Way:** The backend used a web-based redirect flow (Goth/Gothic) with sessions. This is completely broken for mobile apps.
**The New Way:** The backend now strictly uses **stateless, server-side ID token verification** via two new endpoints. 

### What you need to change:
You must integrate native Flutter SDKs to obtain ID tokens directly from Google/Apple, and pass those tokens to the backend.

### 1.1 Google Sign-In
1. Add the `google_sign_in` package to `pubspec.yaml`.
2. When the user taps "Sign in with Google", call the native plugin to authenticate.
3. Extract the `idToken` from the Google authentication object.
4. Send a `POST` request to `/auth/google/token`:
   ```json
   {
     "id_token": "eyJhbGciOiJSUzI1NiIs..."
   }
   ```

### 1.2 Apple Sign-In
1. Add the `sign_in_with_apple` package to `pubspec.yaml`.
2. Call `SignInWithApple.getAppleIDCredential()`.
3. Extract the `identityToken`.
4. **CRITICAL:** Apple only gives you the user's name (`givenName` / `familyName`) on the **very first sign-in**. You must capture it from the UI credential object and send it. The backend will ignore it on subsequent logins if empty.
5. Send a `POST` request to `/auth/apple/token`:
   ```json
   {
     "id_token": "eyJhbGciOiJSUzI1NiIs...",
     "full_name": "Jane Doe" // Send only if available
   }
   ```

### 1.3 Expected Response (Both Endpoints)
On success, the backend returns your standard JWTs. **Update your secure storage and auth state with this data:**
```json
{
  "user": { "id": "uuid", "username": "user", "email": "a@b.com" },
  "access_token": "eyJ...",
  "refresh_token": "eyJ...",
  "expires_in": 900,
  "is_new_user": true // If true, route them to an onboarding/username selection screen!
}
```

---

## 🚨 2. Security Fix: Stop Sending `user_id`

**The Old Way:** You were sending `user_id` in the request body when creating posts. This created a user-impersonation vulnerability.
**The New Way:** The backend now strictly ignores any client-sent `user_id` and securely infers the author from the `Authorization: Bearer <access_token>` header.

### What you need to change:
Refactor your models and API calls to remove `user_id` from payloads.

**Create Post Endpoint:** `POST /api/v1/posts`
**New Payload:**
```json
// Do NOT send user_id here anymore
{
  "content": "This is a post!",
  "media_urls": ["https://..."],
  "latitude": -6.3621,
  "longitude": 106.8271
}
```

---

## 🚨 3. Standardization of Error Responses

**The Old Way:** The backend sometimes leaked internal database errors or had inconsistent formats.
**The New Way:** All errors across the API are now standardized to a single, human-readable format.

### What you need to change:
Ensure your Dio Error Interceptors are looking specifically for the `error` key in HTTP 4xx/5xx responses.

**Format:**
```json
{
  "error": "Human readable error message suitable for UI"
}
```
*Action:* Update your API client to catch this format and throw custom app exceptions, which your UI layers should display in generic SnackBars or Dialogs.

---

## Refactoring Action Plan

Please execute the following updates to the existing Flutter codebase:

1. [ ] **Update Auth Service:** Remove any webview/redirect logic. Implement `google_sign_in` and `sign_in_with_apple` fetching native ID tokens.
2. [ ] **Implement New Endpoints:** Add the Dio calls for `POST /auth/google/token` and `POST /auth/apple/token`.
3. [ ] **Handle Apple First-Login Name:** Ensure your Apple login function captures the `givenName`/`familyName` from the native credential to pass as `full_name`.
4. [ ] **Handle Onboarding:** Check the `is_new_user` boolean in the login response to handle new user onboarding logically (e.g. going to an Edit Profile screen).
5. [ ] **Update Post Models:** Remove `user_id` from the `CreatePostRequest` model and the corresponding Dio POST call to `/api/v1/posts`.
6. [ ] **Review Error Interceptors:** Ensure Dio globally maps HTTP error bodies correctly so `response.data['error']` is cleanly displayed in the UI.

*Begin execution by updating the `pubspec.yaml` with the auth packages, then refactoring your Authentication Repository/Service to use the new endpoints.*
