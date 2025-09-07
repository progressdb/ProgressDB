# @progressdb/react — React bindings for ProgressDB SDK

This package provides a small set of React utilities (Provider + hooks) that wrap the underlying `@progressdb/js` SDK.

Install (after publishing):

  npm install @progressdb/react

Usage

Wrap your app with the provider and use hooks to access messages, threads, and reactions:

```tsx
import React from 'react'
import { ProgressDBProvider, useMessages } from '@progressdb/react'

function ThreadView({ threadId }: { threadId: string }) {
  const { messages, loading, refresh, create } = useMessages(threadId)
  if (loading) return <div>Loading...</div>
  return (
    <div>
      {messages?.map(m => <div key={m.id}>{JSON.stringify(m)}</div>)}
    </div>
  )
}

export default function App() {
  return (
<ProgressDBProvider options={{ baseUrl: 'https://api.example.com', apiKey: 'FRONTEND_KEY' }}>
  <ThreadView threadId="t1" />
</ProgressDBProvider>
  )
}
```

Provided hooks

- `useProgressClient()` — returns the raw `ProgressDBClient` instance
- `useMessages(threadId)` — list messages in a thread; provides `messages`, `loading`, `error`, `refresh`, `create`
- `useMessage(id)` — get a single message; provides `message`, `loading`, `error`, `refresh`, `update`, `remove`
- `useThreads()` — list/create threads
- `useReactions(messageId)` — list/add/remove reactions

Notes

- This package is a lightweight convenience layer; it intentionally keeps logic simple and performs naive refreshes after mutating operations. For production apps you may want to integrate caching strategies (SWR/React-Query) or more granular state updates.
- The package is TypeScript-first and requires React as a peer dependency.

Automatic user signing

The provider accepts an optional `getUserSignature` prop — a function that returns `{ userId, signature }` (can be async). The provider will call this function and attach the returned values to the underlying SDK as `defaultUserId` and `defaultUserSignature`. Example usage:

```tsx
<ProgressDBProvider
  options={{ baseUrl: 'https://api.example.com', apiKey: 'FRONTEND_KEY' }}
  getUserSignature={async () => {
    // Call your backend endpoint which returns { userId, signature }
    const res = await fetch('/auth/sign', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ userId: 'user123' })
    });
    return await res.json();
  }}
>
  <App />
</ProgressDBProvider>
```

Important: Do not call the server `/v1/_sign` endpoint directly from untrusted frontends. The `getUserSignature` callback should call a trusted backend endpoint that holds the admin/backend API key and returns only the signature for the frontend to use. Your backend should authenticate the requester before issuing signatures.
