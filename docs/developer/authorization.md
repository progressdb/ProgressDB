# Authorization — Developer Guide

This document explains the authorization model, role semantics, and handler-level enforcement used by the server. It describes typical policies, enforcement points, and operational guidance for admin and backend operations.

Roles and capability matrix
---------------------------
- `admin`:
  - Full privileges: read/write/delete thread metadata and keys, list and rotate keys, perform rewrap operations.
  - Admin routes require admin role and AdminApiKey security.
- `backend`:
  - Trusted services, able to request signatures for arbitrary user IDs and act on behalf of users.
  - Backend can set author in request bodies or headers for server-side operations.
- `frontend`:
  - End-user clients. Limited capabilities to reduce blast radius: typically `GET|POST /v1/messages`, `GET /healthz`, and thread-scoped read operations when authorized.

Authorization enforcement points
--------------------------------
1. Security middleware (AuthenticateRequestMiddleware)
   - Resolves role (frontend/backend/admin/unauth) based on API key and enforces coarse-grained access (e.g., reject unauthenticated requests except health probes).
   - Writes `X-Role-Name` header for handlers.

2. Signature verification (RequireSignedAuthor)
   - Verifies the author identity via HMAC signature and injects canonical author into context. Handlers consult this for fine-grained authorization.

3. Handler-level checks (ownership & scope)
   - Example checks:
     - Thread operations: only thread author (or admin) may delete or change metadata.
     - Message edits/deletes: allowed only for verified author or admin.
     - Reaction updates: identity-scoped; must match provided reactor identity.

Ownership & verified author
---------------------------
- Canonical author: resolved via `auth.ResolveAuthorFromRequest` which prefers signature-verified author from request context.
- Backend/admin callers: may assert an author via request body/header; but handlers must treat such an override carefully (only allowed for backend/admin roles).

Error semantics
---------------
- Missing authentication (no API key and no signature for protected endpoint): 401 Unauthorized.
- Author mismatch (verified author differs from provided author in body/query): 403 Forbidden.
- Insufficient role (frontend trying to access admin endpoints): 403 Forbidden.

Admin & rotation policies
--------------------------
- Admin endpoints (rotate, rewrap) require admin role. These operations may be long-running and should be executed only during maintenance windows.
- Rotation workflow:
  1. Generate or provide a new KEK.
  2. Rewrap DEKs in the KMS (admin/rewrap_batch) or via admin rewrap tooling.
  3. Validate sample decrypts after rewrap.

Least-privilege guidance
-------------------------
- Issue distinct API keys per role and purpose (frontend, backend, admin).
- Backend services that need signing authority should have specifically-scoped backend keys and rotate them regularly.

Auditing and observability
---------------------------
- Log authorization decisions (role resolved, authorized/forbidden) at info/warn levels as appropriate — redact sensitive headers.
- Capture admin actions (generate_kek, rotate_thread_dek, rewrap_batch) in audit logs.

Developer checklist for adding routes
------------------------------------
1. Decide whether route is admin/backend/frontend-only and document.
2. Ensure middleware ordering is correct (security → signature verification → handler).
3. Use `auth.ResolveAuthorFromRequest(r, bodyAuthor)` in handlers to compute canonical author.
4. Perform ownership checks: compare verified author to resource owner.

