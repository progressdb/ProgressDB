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
const messages = await client.listMessages({ thread: 't1', limit: 50 });
```

## Authentication

- Frontend API key via `X-API-Key` header
- User operations require `X-User-ID` and `X-User-Signature` headers
- Get signatures from your backend: `POST /v1/_sign`

## API

### Client
```ts
new ProgressDBClient(options: SDKOptions)
```

### Messages
- `listMessages(opts)` - List messages
- `createMessage(opts)` - Create message
- `listThreadMessages(threadID, opts)` - List thread messages
- `getThreadMessage(threadID, id)` - Get message
- `updateThreadMessage(threadID, id, opts)` - Update message
- `deleteThreadMessage(threadID, id)` - Delete message
- `listMessageVersions(threadID, id)` - List message versions

### Threads
- `createThread(opts)` - Create thread
- `listThreads(opts)` - List threads
- `getThread(threadID)` - Get thread
- `updateThread(threadID, opts)` - Update thread
- `deleteThread(threadID)` - Delete thread

### Reactions
- `listReactions(threadID, id)` - List reactions
- `addOrUpdateReaction(threadID, id, input)` - Add/update reaction
- `removeReaction(threadID, id, identity)` - Remove reaction

### Health
- `health()` - Check service health

## Types

Key exported types: `Message`, `Thread`, `ReactionInput`, `SDKOptions`, `ProgressDBClient`.

## Development

```bash
npm run build    # Build JS and .d.ts files
npm test         # Run tests
npm pack         # Create tarball for local testing
```

The SDK compiles to JavaScript with TypeScript declarations and uses `fetch`. In Node.js, provide a fetch polyfill.