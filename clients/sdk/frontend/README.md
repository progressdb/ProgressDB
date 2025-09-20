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

Distribution & types

- The frontend SDK package (`@progressdb/js`) is published with compiled JS and TypeScript declaration files (`.d.ts`). Consumers should import both runtime and types from the published package, e.g.:

```ts
import ProgressDBClient, { SDKOptions, Message, Thread, ReactionInput } from '@progressdb/js';
```

- Ensure you build the SDK (`npm run build`) before packing/publishing so `dist/index.d.ts` is present.

React integration

- The React package (`@progressdb/react`) depends on `@progressdb/js` as a runtime + types peer/dependency. Use the `ProgressDBProvider` and hooks to access the SDK from React apps. Example:

```tsx
import React from 'react';
import ProgressDBProvider, { useMessages } from '@progressdb/react';
import ProgressDBClient from '@progressdb/js';

const client = new ProgressDBClient({ baseUrl: 'https://api.example.com', apiKey: 'KEY' });

export default function App(){
  return (
    <ProgressDBProvider options={{ baseUrl: 'https://api.example.com', apiKey: 'KEY' }} getUserSignature={async ()=>({ userId: 'u', signature: 's' })}>
      {/* your app */}
    </ProgressDBProvider>
  );
}
```

Notes on listing threads

- The server supports filtering threads by `author`, `title`, and `slug` via
  query parameters. Backend SDKs expose those filters as `listThreads(opts)`.
- Frontend callers using the signed-author flow should call `listThreads()` and
  rely on the SDK's default `userId`/`userSignature` (or pass them directly);
  the server will resolve the author from the signature.

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
