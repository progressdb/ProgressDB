# @progressdb/js — ProgressDB Frontend SDK (TypeScript)

This lightweight SDK provides typed wrappers for ProgressDB HTTP endpoints described in `service/docs/openapi.yaml`.

Installation (after published):
```bash
npm install @progressdb/js
```

Quick usage

- Import and construct the client with your frontend API key and optionally a default user id/signature:

```ts
import ProgressDBClient from '@progressdb/js';

const client = new ProgressDBClient({
  baseUrl: 'https://api.example.com',
  apiKey: 'FRONTEND_API_KEY',
  defaultUserId: 'user123',
  defaultUserSignature: 'signature-from-backend'
});

// list messages in a thread
const res = await client.listMessages({ thread: 't1', limit: 50 });
```

Types & distribution

- The package publishes compiled JavaScript and TypeScript declaration files (`.d.ts`) so consumers can import both runtime and types from `@progressdb/js`.
- Key exported types: `Message`, `Thread`, `ReactionInput`, `SDKOptions`, and the default `ProgressDBClient`.

Authentication & Signing

- Frontend callers should include a frontend API key (sent as `X-API-Key`).
- User operations require `X-User-ID` and `X-User-Signature` headers. A backend component holding a backend/admin API key must call `POST /v1/_sign` to obtain a signature for a user id; the frontend attaches that signature for subsequent calls.

API surface (high level)

- `new ProgressDBClient(opts: SDKOptions)` — construct the client
- Health: `health()`
- Messages: `listMessages`, `createMessage`
- Thread-scoped message APIs: `listThreadMessages`, `getThreadMessage`, `updateThreadMessage`, `deleteThreadMessage`
- Versions: `listMessageVersions`
- Reactions: `listReactions`, `addOrUpdateReaction(threadID, id, input: ReactionInput)`, `removeReaction(threadID, id, identity: string)`
- Threads: `createThread`, `listThreads`, `getThread`, `updateThread`, `deleteThread`

Notes on listing threads

- The server supports filtering threads by `author`, `title` and `slug` via query parameters. Backend SDKs expose these filters on `listThreads(opts)`.
- Frontend callers should use the signed-author flow: obtain a signature (`POST /v1/_sign`) from your backend and provide `X-User-ID` and `X-User-Signature` headers (the SDK accepts `defaultUserId` / `defaultUserSignature` in `SDKOptions`).

Reactions and types

- The SDK exports `ReactionInput = { id: string; reaction: string }` for use with `addOrUpdateReaction`.
- `removeReaction` expects a reactor identity string and calls `DELETE /v1/threads/{threadID}/messages/{id}/reactions/{identity}`.

Build & publish

1. Build the SDK to emit JS and `.d.ts` files:

```bash
cd clients/sdk/frontend/typescript
npm run build
```

2. Pack or publish the package. For local testing without a registry you can create a tarball and install it into a consuming package:

```bash
npm pack
# install into other package
cd ../reactjs
npm install ../typescript/progressdb-js-*.tgz
```

Local development: if you use a monorepo/workspaces setup, ensure the React package depends on the built `@progressdb/js` package or uses path mappings to resolve `@progressdb/js` to the built `dist` output during development.

Notes

- This SDK is written in TypeScript and compiled to JS for distribution. The bundle is intentionally minimal and uses `fetch`. In Node, provide a global `fetch` polyfill or pass one via `SDKOptions.fetch`.
