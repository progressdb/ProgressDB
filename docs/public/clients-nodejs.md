---
section: clients
title: "Node.js Reference"
order: 1
visibility: public
---

# Node.js Backend SDK

Install the backend SDK:

```bash
npm install @progressdb/node
```

Quickstart

```ts
import ProgressDB from '@progressdb/node'
const db = ProgressDB({ baseUrl: 'https://api.example.com', apiKey: process.env.PROGRESSDB_KEY })

// Sign a user id (backend-only)
const { signature } = await db.signUser('user-123')

// Create thread (backend must provide author)
const thread = await db.createThread({ title: 'General' }, 'service-account')

// Create message (backend must provide author)
const msg = await db.createMessage({ thread: thread.id, body: { text: 'hello' } }, 'service-account')
```

API Surface (selected)

- `new BackendClient({ baseUrl, apiKey })`
- `signUser(userId)` — POST `/v1/_sign`
- `adminHealth()`, `adminStats()` — admin endpoints
- `listThreads(opts?)`, `getThread(id, author)`, `createThread(thread, author)`, `deleteThread(id, author)`
- Message APIs: `createMessage`, `listThreadMessages`, `updateThreadMessage`, etc.

Notes

- Backend SDKs hold admin keys and must run on trusted servers only.
- Use `signUser` to obtain a signature for frontend clients (do not call `/v1/_sign` from the browser).

Error handling & retries

- The SDK throws `ApiError` for non-2xx responses exposing `.status` and `.body`.
- Retry transient 5xx errors with exponential backoff (configurable via options).

Headers & auth

- Backend clients should send `Authorization: Bearer <key>` or `X-API-Key: <key>`.
- When using backend keys, supply an explicit `author` on admin operations or
  set `X-User-ID` header. Frontend flows should obtain signatures via `signUser`.

Files & contribution

- The SDK source lives under `clients/sdk/backend/nodejs/` in the repo; see
  `README.md` there for more details on API surface and development.
