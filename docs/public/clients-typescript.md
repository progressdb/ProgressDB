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

## Recommended API surface

```ts
// Factory
new FrontendClient(options: { baseUrl: string; fetch?: typeof fetch }): FrontendClient

// Thread APIs
listThreads(opts?: { author?: string; title?: string; slug?: string }): Promise<Thread[]>
getThread(id: string): Promise<Thread>

// Message APIs
listMessages(threadId: string, opts?: { limit?: number; before?: string }): Promise<Message[]>
createMessage(message: Partial<Message>, signer: { userId: string; signature: string }): Promise<Message>

// Thread creation
createThread(input: { title: string }, signer: { userId: string; signature: string }): Promise<Thread>
```

Notes

- The frontend SDK must not contain backend keys. Use the signing flow: your backend calls `/v1/_sign` and returns the signature to the client.
- The SDK uses `fetch` and accepts a custom `fetch` implementation for non-browser runtimes.

See `clients/sdk/frontend/README.md` for suggested types, React integration, and build notes.
