---
section: others
title: "Integrating with Vercel AI SDK"
order: 1
visibility: public
---

# Vercel AI SDK Integration

Build chat experiences with ProgressDB + Vercel AI SDK.

ProgressDB supports all Vercel AI SDK persistence structures.
You simply need to decide where you want to call for the storing of your messages.
- See [Vercel AI chat persistence docs](https://ai-sdk.dev/docs/ai-sdk-ui/chatbot-message-persistence) for options.

## Setup

1. Run ProgressDB locally or use hosted instance
2. Create backend endpoint to sign users with backend SDK

## Flow

1. User sends message to the response streaming endpoint  
   - *Store it in ProgressDB & it will ack in sub millisecond speeds, no overhead*
2. Generate response with Vercel AI SDK
3. Save AI response to ProgressDB  
   - *Store it in ProgressDB & it will ack in sub millisecond speeds, no overhead*

## Node.js Backend Example

```ts
import ProgressDB from '@progressdb/node'

const db = ProgressDB({ 
  baseUrl: 'https://api.example.com', 
  apiKey: process.env.PROGRESSDB_KEY 
})

// API route to sign users
export async function POST(req: Request) {
  const { userId } = await req.json()
  const signature = await db.signUser(userId)
  return Response.json({ userId, signature })
}

// Save messages from Vercel AI SDK
export async function POST(req: Request) {
  const { messages, chatId } = await req.json()
  
  // Save to ProgressDB thread
  await db.createThreadMessage(chatId, {
    body: { text: messages[messages.length - 1].content }
  }, 'user-123')
  
  // Generate AI response with Vercel AI SDK...
  // Then store the AI response with ProgressDB SDK again or frontend option below 👇
}
```

## React Frontend Example

```tsx
import React from 'react';
import ProgressDBProvider, { useMessages } from '@progressdb/react';
import { useChat } from 'ai/react';

function ChatInterface({ threadId }: { threadId: string }) {
  const { messages, loading, create } = useMessages(threadId);
  const { sendMessage } = useChat({
    api: '/api/chat',
    onFinish: async (message) => {
      // Save AI response to ProgressDB
      await create({
        body: { text: message.content },
        role: 'assistant'
      });
    }
  });

  return (
    <div>
      {messages.map(m => (
        <div key={m.id}>{m.body.text}</div>
      ))}
      <button onClick={() => sendMessage('Hello')}>Send</button>
    </div>
  );
}

export default function App() {
  return (
    <ProgressDBProvider
      options={{ baseUrl: 'https://api.example.com', apiKey: 'FRONTEND_KEY' }}
      getUserSignature={async () => {
        const res = await fetch('/api/sign-user', { 
          method: 'POST', 
          body: JSON.stringify({ userId: 'user123' }) 
        });
        return res.json();
      }}
    >
      <ChatInterface threadId="general" />
    </ProgressDBProvider>
  );
}
```

## SDKs & their methods available:

- [TypeScript SDK](/clients/typescript)
- [React SDK](/clients/reactjs)
- [Python SDK](/clients/python)
- [Node.js SDK](/clients/nodejs)