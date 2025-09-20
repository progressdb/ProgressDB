# @progressdb/react — React Provider & Hooks

This package provides a React-friendly wrapper around the ProgressDB frontend SDK (`@progressdb/js`). It exposes a `ProgressDBProvider` that initialises a `ProgressDBClient` and a set of hooks (`useMessages`, `useThreads`, `useReactions`, `useMessage`, etc.) to interact with ProgressDB from React components.

Installation (after published):

```bash
npm install @progressdb/react @progressdb/js
```

Quick usage

```tsx
import React from 'react';
import ProgressDBProvider, { useMessages } from '@progressdb/react';

function MessagesView({ threadId }: { threadId: string }){
  const { messages, loading, refresh, create } = useMessages(threadId);
  // ... render messages
}

export default function App(){
  return (
    <ProgressDBProvider
      options={{ baseUrl: 'https://api.example.com', apiKey: 'FRONTEND_API_KEY' }}
      getUserSignature={async () => ({ userId: 'user123', signature: 'signature' })}
    >
      <MessagesView threadId="t1" />
    </ProgressDBProvider>
  );
}
```

Notes

- The provider requires a `getUserSignature` callback that returns `{ userId, signature }` (can be async). This function should call your backend which in turn calls the admin signing endpoint (`/v1/_sign`).
- The React package imports runtime and types from `@progressdb/js`; install the SDK package so TypeScript resolves the exported types (including `ReactionInput`).

