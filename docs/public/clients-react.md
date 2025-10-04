---
section: clients
title: "React Reference"
order: 4
visibility: public
---

# React SDK

Install the React bindings:

```bash
npm install @progressdb/react
```

Example

```jsx
import React from 'react'
import { ProgressDBProvider, useMessages } from '@progressdb/react'

function Chat({ threadId }) {
  const { messages, loading, create } = useMessages(threadId)
  if (loading) return <div>Loading…</div>
  return (
    <div>
      <ul>{messages?.map(m => <li key={m.id}>{m.body?.text}</li>)}</ul>
      <button onClick={() => create({ body: { text: 'Hi from React!' } })}>Send</button>
    </div>
  )
}

export default function App(){
  return (
    <ProgressDBProvider options={{ baseUrl: 'http://localhost:8080', apiKey: 'pk_frontend' }} getUserSignature={async ()=>({ userId: 'u', signature: 's' })}>
      <Chat threadId="general" />
    </ProgressDBProvider>
  )
}
```

API surface (hooks)

- `useMessages(threadId)` — returns `{ messages, loading, create, list, update, remove }`.
- `useThreads()` — thread list and create helpers.
- `ProgressDBProvider` props:
  - `options`: `{ baseUrl, apiKey, fetch? }` — `apiKey` should be a frontend/public key.
  - `getUserSignature`: async function that returns `{ userId, signature }` for the current user.

Auth flow

- The React SDK relies on the signing flow: backends call `POST /v1/_sign` (using a backend key) and return a `signature` for a `userId` to the client. The client then includes `X-User-ID` and `X-User-Signature` headers for protected operations.

Development notes

- The React bindings are lightweight and built on top of the frontend TypeScript SDK (`@progressdb/js`). See `clients/sdk/frontend/README.md` for details.

