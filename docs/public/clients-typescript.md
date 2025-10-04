---
section: clients
title: "TypeScript Reference"
order: 3
visibility: public
---

# TypeScript / JavaScript SDK

This page covers the TypeScript SDK for browser and server-side usage.

Install:

```bash
npm install @progressdb/js
```

Usage (browser):

```js
import ProgressDBClient from '@progressdb/js'
const client = new ProgressDBClient({ baseUrl: 'http://localhost:8080', apiKey: 'pk_frontend' })
```

Auth: frontends must obtain a user signature from a trusted backend and set
`X-User-ID` and `X-User-Signature` on requests to protected endpoints.

Recommended API surface

- `new FrontendClient({ baseUrl, fetch? })` — factory for browser-safe client.
- `listThreads(opts?)` — lists threads; supports filters `author`, `title`, `slug`.
- `getThread(id)` — retrieve thread metadata.
- `listMessages(threadId, opts?)` — list messages in a thread.
- `createMessage(message, { userId, signature })` — create a message using a signed user.
- `createThread({ title }, { userId, signature })` — create a thread using a signed user.

Notes

- The frontend SDK must not contain backend keys. Use the signing flow: your backend calls `/v1/_sign` and returns the signature to the client.
- The SDK uses `fetch` and accepts a custom `fetch` implementation for non-browser runtimes.

See `clients/sdk/frontend/README.md` for suggested types, React integration, and build notes.

