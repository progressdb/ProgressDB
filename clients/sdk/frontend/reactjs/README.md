# @progressdb/react

ProgressDB React SDK with provider and hooks.

## Installation

```bash
npm install @progressdb/react @progressdb/js
```

## Quick Start

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

## API

### Provider
- `ProgressDBProvider` - Wrap app with provider

### Hooks
- `useProgressClient()` - Get client instance
- `useUserSignature()` - Get user signature
- `useMessages(threadKey, query)` - List messages in thread
- `useMessage(threadKey, messageKey)` - Get single message
- `useThreads(query)` - List threads
- `useHealthz()` - Basic health check
- `useReadyz()` - Readiness check with version