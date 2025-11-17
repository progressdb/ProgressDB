# @progressdb/js

ProgressDB TypeScript SDK for frontend applications.

## Installation

```bash
npm install @progressdb/js
```

## Quick Start

```ts
import ProgressDBClient from '@progressdb/js';

const client = new ProgressDBClient({
  baseUrl: 'https://api.example.com',
  apiKey: 'FRONTEND_API_KEY',
  defaultUserId: 'user123',
  defaultUserSignature: 'signature-from-backend'
});

// List messages in a thread
const messages = await client.listThreadMessages('t1', { limit: 50 });
```

## Authentication

- Frontend API key via `X-API-Key` header
- User operations require `X-User-ID` and `X-User-Signature` headers
- Use the backend SDK or sign endpoint to securely generate signatures for your users

## API

### Client
```ts
new ProgressDBClient(options: SDKOptions)
```

### Messages
- `listThreadMessages(threadKey, query)` - List messages in thread
- `createThreadMessage(threadKey, message)` - Create message
- `getThreadMessage(threadKey, messageKey)` - Get message
- `updateThreadMessage(threadKey, messageKey, message)` - Update message
- `deleteThreadMessage(threadKey, messageKey)` - Delete message

### Threads
- `createThread(thread)` - Create thread
- `listThreads(query)` - List threads
- `getThread(threadKey)` - Get thread
- `updateThread(threadKey, thread)` - Update thread
- `deleteThread(threadKey)` - Delete thread

### Health
- `healthz()` - Basic health check
- `readyz()` - Readiness check with version

