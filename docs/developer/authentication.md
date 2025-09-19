# Authentication — Developer Guide

This document describes the authentication mechanisms used by the server, the header/API semantics, middleware ordering, and guidance for development and operations.

Overview
--------
- The server supports two main authentication flows:
  1. API Key-based (backend/admin/backend scopes)
  2. Signature-based (frontend) — HMAC-SHA256 signatures of a user id
- Middleware enforces access rules and verifies identities before handler logic runs.
- Authentication is separate from authorization: auth verifies who you are; authorization decides what you can do.

Headers & API surface
----------------------
- `X-API-Key` or `Authorization: Bearer <key>`
  - Backend/admin keys are long-lived API keys placed in server runtime config.
  - Keys are assigned roles (frontend, backend, admin) via server config and used to determine caller scope.
- `X-User-ID` and `X-User-Signature`
  - Frontend callers use a signature flow: a backend signer issues an HMAC-SHA256 signature for a `userId` using a backend API key as secret; the client attaches `X-User-ID` and `X-User-Signature` to requests.
  - The signature verification middleware validates the signature against configured signing keys and derives the canonical author identity.
- `X-Identity` (used for reactions): optional opaque identity id for reaction authors.

Roles & Scopes
--------------
- `frontend`:
  - Intended for end-user clients. Limited scope: `GET|POST /v1/messages` and `GET /healthz` by default.
  - Must use signature-based author flow when creating user-scoped data.
- `backend`:
  - Trusted services. Can call admin-like endpoints and issue signatures for users.
  - May omit `X-User-Signature` when providing `X-User-ID` in backend contexts.
- `admin`:
  - Full privileges, can read raw keys and perform rotation.

Middleware ordering & responsibilities
------------------------------------
The server registers middleware in an order that enforces practical checks early and avoids leaking sensitive operations:

1. Security middleware (`AuthenticateRequestMiddleware`)
   - Performs API-key parsing (Authorization / X-API-Key), rate limiting, IP whitelist, CORS handling, and early allow/deny checks.
   - Exposes a role string via `X-Role-Name` for downstream handlers.
   - Important: this middleware will not reject requests that present a frontend signature header pair (X-User-ID + X-User-Signature). Those are allowed through so the signature middleware can verify and resolve the canonical author.

2. Signature middleware (`RequireSignedAuthor`)
   - Verifies HMAC-SHA256 signature of `X-User-ID` using configured signing keys (runtime signing keys derived from backend keys).
   - Rejects requests when signature is missing/invalid for frontends. For backend/admin roles a missing signature is allowed (backend may assert `author` via body/header).
   - On success, it injects the verified author into request context (used by handlers and other auth checks).

3. Handler-level checks
   - Handlers use `auth.ResolveAuthorFromRequest` to derive the canonical author (from signature, header, or body) and to decide ownership.

Signature flow (HMAC signing)
-----------------------------
1. A backend caller requests a signature for a `userId` by calling `POST /v1/_sign` with JSON `{ "userId": "..." }` and a backend API key.
2. Server verifies caller role is `backend` and computes HMAC-SHA256(userId, key) and returns hex signature.
3. Client attaches to subsequent requests:
   - `X-User-ID: <userId>`
   - `X-User-Signature: <hex-hmac>`
4. Signature middleware recomputes expected HMAC using configured signing keys and compares with provided signature using constant-time compare.

Authorization basics
--------------------
- Handlers enforce ownership and role checks:
  - For user-scoped resources, owner == verified author derived from signature or backend-provided author.
  - Admin or backend role can override author via query parameter `author` or body when explicitly allowed.
- Common checks:
  - `author must match verified author` → 403 Forbidden
  - `missing signature for frontend` → 401 Unauthorized

Edge cases & details
--------------------
- Backend callers with a backend API key may call signing endpoint to obtain signatures for users; use this flow for server-to-client bootstrap.
- `key_present` logging confusion: server logs `has_api_key` to indicate an API key header was present (true only when Authorization or X-API-Key was supplied). We intentionally avoid logging actual keys.
- `RequireSignedAuthor` will refuse to run if no signing keys are configured; ensure backend signing keys are set in runtime config for the signature flow to work.
- For admin endpoints, require AdminApiKey security in OpenAPI and middleware will check role.

Examples
--------
- Request a signature (backend):
  - POST /v1/_sign with `Authorization: Bearer <backend-key>` and body `{ "userId": "user1" }`.
- Use signed request (client):
  - GET /v1/messages with headers `X-User-ID: user1` and `X-User-Signature: <hex-hmac>`.

Security recommendations
------------------------
- Keep backend/admin API keys secret and rotate periodically.
- Use short-lived signing tokens or allow backends to issue ephemeral signatures if you need extra safety.
- Enforce strict CORS for frontend use and IP whitelists for admin operations.

Debugging tips
--------------
- To debug signature verification failures: check which signing keys are configured in server runtime; enable debug logs for `signature_verified` and `invalid_signature` events.
- For admin/role issues: inspect `X-Role-Name` header emitted by security middleware in logs to see resolved role.

