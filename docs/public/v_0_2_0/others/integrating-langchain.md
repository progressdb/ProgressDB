---
section: others
title: "Integrating with LangChain"
order: 2
visibility: public
---

# LangChain Integration

Build AI applications with ProgressDB + LangChain.

ProgressDB serves as persistent memory for LangChain conversations, enabling chat history and context retention.

## Setup

1. Run ProgressDB locally or use hosted instance
2. Create backend endpoint to sign users with backend SDK
3. Use ProgressDB as LangChain memory store
4. Integrate with LangChain chains and agents

## Python Backend Example

```python
from progressdb import ProgressDBClient
from langchain.memory import ChatMessageHistory
from langchain.chat_models import ChatOpenAI
from langchain.chains import ConversationChain
from langchain.schema import HumanMessage, AIMessage

db = ProgressDBClient(
    base_url="https://api.example.com",
    api_key="your-api-key"
)

class ProgressDBMemory:
    def __init__(self, thread_id: str, user_id: str):
        self.thread_id = thread_id
        self.user_id = user_id
    
    def load_memory_variables(self):
        messages = db.list_thread_messages(self.thread_id, user_id=self.user_id)
        history = ChatMessageHistory()
        
        for msg in messages:
            if msg.role == "user":
                history.add_user_message(msg.body["text"])
            else:
                history.add_ai_message(msg.body["text"])
        
        return {"chat_history": history}
    
    def save_context(self, inputs, outputs):
        db.create_thread_message(
            self.thread_id,
            {"body": {"text": inputs["input"]}, "role": "user"},
            self.user_id
        )
        
        db.create_thread_message(
            self.thread_id,
            {"body": {"text": outputs["response"]}, "role": "assistant"},
            self.user_id
        )

# FastAPI endpoint
@app.post("/chat")
async def chat(request: ChatRequest):
    memory = ProgressDBMemory(request.thread_id, request.user_id)
    llm = ChatOpenAI(temperature=0)
    chain = ConversationChain(llm=llm, memory=memory)
    
    response = chain.run(input=request.message)
    return {"response": response}
```

## React Frontend Example

```tsx
import React, { useState } from 'react';
import ProgressDBProvider, { useMessages } from '@progressdb/react';

function ChatInterface({ threadId }: { threadId: string }) {
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const { messages, create } = useMessages(threadId);

  const sendMessage = async () => {
    if (!input.trim()) return;
    
    setLoading(true);
    
    // Save user message
    await create({
      body: { text: input },
      role: 'user'
    });
    
    // Get AI response from LangChain backend
    const response = await fetch('/api/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        message: input,
        threadId,
        userId: 'user123'
      })
    });
    
    const data = await response.json();
    
    // Save AI response
    await create({
      body: { text: data.response },
      role: 'assistant'
    });
    
    setInput('');
    setLoading(false);
  };

  return (
    <div>
      <div>
        {messages.map(m => (
          <div key={m.id}>
            <strong>{m.role}:</strong> {m.body.text}
          </div>
        ))}
      </div>
      
      <div>
        <input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="Type a message..."
          onKeyPress={(e) => e.key === 'Enter' && sendMessage()}
        />
        <button onClick={sendMessage} disabled={loading}>
          {loading ? 'Sending...' : 'Send'}
        </button>
      </div>
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
      <ChatInterface threadId="langchain-chat" />
    </ProgressDBProvider>
  );
}
```

## SDKs & their methods available:

- [TypeScript SDK](/clients/typescript)
- [React SDK](/clients/reactjs)
- [Python SDK](/clients/python)
- [Node.js SDK](/clients/nodejs)