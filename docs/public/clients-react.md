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

