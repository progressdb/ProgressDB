# ProgressDB Backend TypeScript SDK (Planning)

Purpose
- Lightweight TypeScript SDK for backend callers (trusted services, jobs, server-side apps) to call ProgressDB admin/backend endpoints.

Audience
- Backend engineers and server-side services that hold backend API keys.

Goals
- Provide simple, typed wrappers around key backend endpoints.
- Manage `Authorization: Bearer <key>` and `X-API-Key` usage consistently.
- Provide `signUser(userId)` helper that calls `/v1/_sign` and returns `{ userId, signature }`.
- Convenience helpers for admin tasks (`adminHealth`, `adminStats`, `listThreads`, `deleteThread`).
- Minimal runtime dependencies; publishable as an npm package.

Non-goals
- Client-side usage or storage of API keys in the browser.
- Full-featured ORM — only thin wrappers around HTTP routes.

API surface (suggested)
- `new BackendClient({ baseUrl: string, apiKey: string, timeoutMs?: number })`
- `signUser(userId: string): Promise<{ userId: string, signature: string }>`
- `adminHealth(): Promise<{status:string, service?:string}>`
- `adminStats(): Promise<{ threads: number, messages: number }>`
-- `listThreads(opts?: { author?: string; title?: string; slug?: string }): Promise<Thread[]>`

- `author` is important for backend calls: the server requires an author to be
  resolved (via signature, `X-User-ID` header, or the `author` query param). For
  GET collection endpoints (no request body) backend callers frequently supply
  `author` as a query parameter or set `X-User-ID` on the request.
- `deleteThread(id: string): Promise<void>`
- `request<T>(method, path, opts?)` — low-level wrapper

Types (core)
- `Message`, `Thread` shapes (keep in `types.ts` and align with API models)

Auth / Headers
- Prefer `Authorization: Bearer <key>`; fallback to `X-API-Key`.
- `signUser` uses the backend key to call `/v1/_sign`.

Error handling
- Throw typed errors `ApiError { status, body }`.
- Retry transient 5xx with exponential backoff (configurable).

File layout (suggested)
- `clients/sdk/backend/`
  - `src/client.ts` (BackendClient)
  - `src/types.ts`
  - `src/http.ts` (fetch wrapper + retries)
  - `README.md` (this file)
  - `package.json`, `tsconfig.json`

Milestones / Roadmap
- M1: scaffold, implement Http wrapper + signUser + admin endpoints.
- M2: add retries, ApiError, tests.
- M3: docs + examples, release v0.1.0.

## Client API notes

- `listThreads(opts?: { author?: string; title?: string; slug?: string }): Promise<Thread[]>` — list threads with optional filters. Backend callers should provide `author` when using backend/admin keys.

- `getThread(id: string, opts?: { author?: string }): Promise<Thread>` — retrieve thread metadata (title, slug, author, timestamps). For backend callers, include `opts.author` or set `X-User-ID` to satisfy server author resolution.
