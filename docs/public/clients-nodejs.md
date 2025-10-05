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

## API surface (selected)

```ts
// Factory
new BackendClient(options: { baseUrl: string; apiKey: string }): BackendClient

// Low-level request helper
request<T>(method: string, path: string, body?: any): Promise<T>

// Signing (backend keys only)
signUser(userId: string): Promise<{ userId: string; signature: string }>

// Admin helpers
adminHealth(): Promise<{ status: string; service?: string }>
adminStats(): Promise<{ threads: number; messages: number }>

// Thread APIs
listThreads(opts?: { author?: string; title?: string; slug?: string }): Promise<Thread[]>
getThread(id: string, author?: string): Promise<Thread>
createThread(thread: Partial<Thread>, author: string): Promise<Thread>
deleteThread(id: string, author: string): Promise<void>

// Thread-scoped message APIs
createMessage(m: Partial<Message>, author: string): Promise<Message>
listThreadMessages(threadID: string, opts?: { limit?: number }, author?: string): Promise<{ thread?: string; messages: Message[] }>
getThreadMessage(threadID: string, id: string, author?: string): Promise<Message>
updateThreadMessage(threadID: string, id: string, msg: Partial<Message>, author?: string): Promise<Message>
deleteThreadMessage(threadID: string, id: string, author?: string): Promise<void>
```

Versions & reactions

- `listMessageVersions(threadID, id, author?)` — GET `/v1/threads/{threadID}/messages/{id}/versions`.
- `listReactions(threadID, id, author?)`, `addOrUpdateReaction(threadID, id, input, author?)`, `removeReaction(threadID, id, identity, author?)`.

Errors & retries

- The SDK throws `ApiError` for non-2xx responses with `.status` and `.body`.
- Transient 5xx errors are retried with exponential backoff (configurable by `maxRetries`).

Auth and signing flow

- Backend SDKs hold secret backend keys and must run on trusted servers only.
- To authenticate a frontend user:
  1. Call `signUser(userId)` on the backend using a backend key.
  2. Return `{ userId, signature }` to the client via your authenticated endpoint.
  3. Client attaches `X-User-ID` and `X-User-Signature` to requests.

Development

- Source: `clients/sdk/backend/nodejs/src/` — build with `npm run build`.
- Tests: `npm test` (see `clients/sdk/backend/nodejs/tests/`).

See `clients/sdk/backend/nodejs/README.md` for complete details and examples.
