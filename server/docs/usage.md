Auth & signing usage
====================

Overview
--------

This server uses HMAC signing of author identifiers (user IDs) for client SDKs.
Backend API keys are used both to request signatures and to verify signatures on
subsequent API calls.

How it works
------------

- Backend service (or any holder of a backend API key) requests a signature for
  a user id by calling `POST /v1/_sign` with JSON body `{ "userId": "..." }`.
- The request must include a valid backend API key (via `Authorization: Bearer <key>`
  or `X-API-Key: <key>`). The security middleware ensures only backend keys may call.
- The server responds with `{ "userId": "...", "signature": "<hex-hmac>" }`.
- Clients attach the following headers on protected requests:
  - `X-User-ID: <userId>`
  - `X-User-Signature: <hex-hmac>`
- The server middleware verifies the signature using the backend API keys configured
  for the server. If verification succeeds the request is allowed and handlers
  derive the author from the verified identity (context).

Notes
-----

- Backend API keys are configured via environment variables or config file the same
  way as other API keys (e.g. `PROGRESSDB_API_BACKEND_KEYS`). The auth middleware
  uses the same backend key set as the security middleware so signing and verification
  share a single source of truth.
- If you rotate backend keys, the server will accept any configured backend key for verification.
- To add expiry or revocation, extend the signed payload (e.g. include a timestamp) and
  adapt the middleware and signing endpoint accordingly.

