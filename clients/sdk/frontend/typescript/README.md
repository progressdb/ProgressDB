# @progrssdb/js — ProgressDB Frontend SDK (TypeScript)

This lightweight SDK provides typed wrappers for ProgressDB HTTP endpoints described in `server/docs/openapi.yaml`.

Installation (after published):
```bash
npm install @progressdb/js
```
Usage

- Import and construct the client with your frontend API key and optionally a default user id/signature:

```ts
import ProgressDBClient from '@progrssdb/js';

const client = new ProgressDBClient({
  baseUrl: 'https://api.example.com',
  apiKey: 'FRONTEND_API_KEY',
  defaultUserId: 'user123',
  defaultUserSignature: 'signature-from-backend'
});

// list messages in a thread
const res = await client.listMessages({ thread: 't1', limit: 50 });
```

Authentication & Signing

- Frontend callers should include a frontend API key (sent as `X-API-Key`).
- User operations require `X-User-ID` and `X-User-Signature` headers. A backend component holding a backend/admin API key must call `POST /v1/_sign` to obtain a signature for a user id; the frontend attaches that signature for subsequent calls.

Offered methods (high level)

- `health()` — Health check
- Messages: `listMessages`, `createMessage`, `getMessage`, `updateMessage`, `deleteMessage`, `listMessageVersions`
- Reactions: `listReactions`, `addOrUpdateReaction`, `removeReaction`
- Threads: `createThread`, `listThreads`, `getThread`, `deleteThread`
- Thread messages: `createThreadMessage`, `listThreadMessages`, `getThreadMessage`, `updateThreadMessage`, `deleteThreadMessage`
- `signUser(userId)` — Admin-only signer endpoint (requires admin API key)

Notes

- This SDK is written in TypeScript and compiled to JS for distribution. The bundle is intentionally minimal and uses `fetch`. In Node, provide a global `fetch` polyfill or pass one via `SDKOptions.fetch`.

