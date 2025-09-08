# @progressdb/node

Node.js TypeScript SDK for backend callers of ProgressDB. Thin, typed wrappers for admin/backend
operations (signing, admin health/stats, thread and message management).

Installation

```bash
npm install @progressdb/node
```

Quickstart

```ts
import ProgressDB from '@progressdb/node'
const db = ProgressDB({ baseUrl: 'https://api.example.com', apiKey: process.env.PROGRESSDB_KEY })

// Sign a user id (backend-only)
const { signature } = await db.signUser('user-123')

// Create thread
const thread = await db.createThread({ title: 'General' })

// Create message
const msg = await db.createMessage({ thread: thread.id, body: { text: 'hello' } })
```

Available methods (BackendClient)

`request<T>(method: string, path: string, body?: any): Promise<T>`

- Low-level HTTP helper. Uses `Authorization: Bearer <apiKey>`.
- `path` must start with `/` (e.g. `/v1/threads`).

`signUser(userId: string): Promise<{ userId: string; signature: string }>`

- Calls `POST /v1/_sign` to obtain an HMAC signature for `userId`.
- Use this signature on the client as `X-User-Signature` alongside `X-User-ID`.

`adminHealth(): Promise<{ status: string; service?: string }>`

- Calls `GET /admin/health` (admin/backend keys only).

`adminStats(): Promise<{ threads: number; messages: number }>`

- Calls `GET /admin/stats` (admin/backend keys only).

`listThreads(): Promise<Thread[]>`

- Calls `GET /v1/threads` and returns the `threads` array.

`createThread(t: Partial<Thread>): Promise<Thread>`

- Calls `POST /v1/threads`. Server assigns `id`, `author`, `slug`, and timestamps.

`deleteThread(id: string): Promise<void>`

- Calls `DELETE /v1/threads/{id}`.

`createMessage(m: Partial<Message>): Promise<Message>`

- Calls `POST /v1/messages`. Server generates the message `id` and sets `author` from the verified signature.

Types

- `Message`: { id, thread, author, ts, body?, reply_to?, deleted?, reactions? }
- `Thread`: { id, title, author, slug?, created_ts?, updated_ts? }

Errors & behavior

- `ApiError` is thrown for non-2xx responses and exposes `.status` and `.body` containing parsed server JSON or raw text.
- Network/timeouts: the SDK retries transient failures with exponential backoff (configurable via `maxRetries`).

HTTP runtime notes

- The SDK uses the global `fetch` API. Node 18+ includes fetch by default. For older Node versions, polyfill `fetch` (e.g., `node-fetch` or `undici`).

Security & signing flow

- The backend SDK holds the backend API key and must run only on trusted servers. To authenticate front-end users, your server should:
  1. Call `signUser(userId)` (server-side) to get a signature.
  2. Return the signature to the client via your own authenticated endpoint.
  3. Client attaches `X-User-ID` and `X-User-Signature` to protected requests.

Development

- Build: `npm run build` (generates `dist/` from `src/`)

Next steps / recommendations

- Add higher-level message APIs (`listMessages`, `getMessage`, `listMessageVersions`, reactions). I can implement these next.
- Add a `fetch` override option to the factory for custom runtimes and tests.
- Add unit tests and a CI workflow.

