# @progressdb/node

Node.js TypeScript SDK for backend callers of ProgressDB. Wraps frontend SDK with automatic signature generation.

## Installation

```bash
npm install @progressdb/node
```

## Quick Start

```ts
import ProgressDB from '@progressdb/node'
const db = ProgressDB({ 
  baseUrl: 'https://api.example.com', 
  apiKey: process.env.PROGRESSDB_KEY 
})

await db.createThread({ title: 'General' }, 'user-123')
```

## API

### Client
```ts
ProgressDB(options: BackendClientOptions)
```

### Messages
- `listThreadMessages(threadKey, query, userId)` - List messages in thread
- `createThreadMessage(threadKey, message, userId)` - Create message
- `getThreadMessage(threadKey, messageKey, userId)` - Get message
- `updateThreadMessage(threadKey, messageKey, message, userId)` - Update message
- `deleteThreadMessage(threadKey, messageKey, userId)` - Delete message

### Threads
- `createThread(thread, userId)` - Create thread
- `listThreads(query, userId)` - List threads
- `getThread(threadKey, userId)` - Get thread
- `updateThread(threadKey, thread, userId)` - Update thread
- `deleteThread(threadKey, userId)` - Delete thread

### Health
- `healthz()` - Basic health check
- `readyz()` - Readiness check with version

### Signature
- `signUser(userId)` - Generate signature for user
- `clearSignatureCache(userId?)` - Clear signature cache
- `getCacheStats()` - Get cache statistics
