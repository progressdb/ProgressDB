# ProgressDB Frontend TypeScript SDK (Planning)

Purpose
- Small TypeScript SDK for browser/front-end apps to interact with ProgressDB endpoints safely.

Audience
- Frontend engineers building SPAs in TypeScript.

Goals
- Provide typed helpers for common user flows: `listThreads`, `getThread`, `listMessages`, `createMessage`, `react`, `removeReaction`.
- Make the signing flow explicit: frontend must obtain `X-User-Signature` from a trusted backend and attach `X-User-ID` + `X-User-Signature` to protected requests.
- Small footprint and zero backend-key handling.

Auth flow (recommended)
1. Client authenticates user (app's own auth) and knows `userId`.
2. Client calls the app backend (e.g., `POST /api/sign`) which uses the Backend SDK to call `/v1/_sign` and returns `{ signature }`.
3. Client calls ProgressDB endpoints via the Frontend SDK and sets headers `X-User-ID` and `X-User-Signature`.

API surface (suggested)
- `new FrontendClient({ baseUrl: string, fetch?: typeof fetch })`
- `listThreads(opts?)`
- `getThread(id)`
- `listMessages(threadId, opts?)`
- `createMessage(message, { userId, signature })`
- `createThread({ title }, { userId, signature })`

Types
- Reuse `Message` and `Thread` types (can import from shared types if published later).

Security considerations
- Never ship backend API keys to the browser.
- Do not call `/v1/_sign` directly from the client — only your backend should call it.

File layout (suggested)
- `clients/sdk/frontend/`
  - `src/client.ts` (FrontendClient)
  - `src/types.ts`
  - `README.md` (this file)
  - `examples/` (React usage)

Milestones
- M1: scaffold, implement core methods and auth header helpers.
- M2: examples and tests.

