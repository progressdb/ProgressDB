
![ProgressDB Logo](/docs/images/logo-colors.png)

ProgressDB is a fast, purpose-built, chat-native database for AI chat threads. The project includes a database service, backend SDKs (Node, Python), and frontend SDKs (TypeScript, React). This quickstart shows how to run the service locally, install the SDKs, and perform basic operations.

> ProgressDB is currently under active development and not yet recommended for production use or full capacity use. 
> The database is heavily tested and actively used internally at [Fathom4Sight](https://fathom4sight.com), but breaking changes may still occur and some features remain incomplete.  
> The next milestone release, v0.5.0, is expected around October 17, with another update planned by the end of the month.  
> A built-in Progressor feature automatically performs necessary database migrations when model definitions change or new things needed. 
> However, this currently applies only to the database layer itself, not the sdks etc.

## Why ProgressDB?

ProgressDB is built specifically for chat threads and makes common chat workflows simple and fast:

- Start a thread by sending a message — the database will auto-create the thread for you.
- Built-in message versioning, edits, replies, reactions, and soft-deletes.
- Optimized for fast threaded message retrievals and common chat patterns.
- Straightforward encryption and API-key based access controls.
- Ship quickly: small service, simple APIs, and SDKs for Python, Node and frontend use.

ProgressDB removes friction when building chat-first apps or features: fewer transformation layers, direct APIs for threads/messages, and tooling to get you from prototype to production faster.

## Quickstart — Docker Image

If you prefer Docker, pull the official image from Docker Hub and run it:

```sh
docker pull docker.io/progressdb/progressdb:latest
docker run -d \
  --name progressdb \
  -p 8080:8080 \
  -v $PWD/data:/data \
  docker.io/progressdb/progressdb --db /data/progressdb
```

This will expose the same endpoints as the binary below.

[![Docker Pulls](https://img.shields.io/docker/pulls/progressdb/progressdb?logo=docker)](https://hub.docker.com/r/progressdb/progressdb)


## Quickstart — Binary Executable

Want to try ProgressDB in seconds?  Download a prebuilt release — no Go toolchain required.

1) Get a binary

   Visit the Releases page and download the binary for your platform:

   https://github.com/ha-sante/ProgressDB/releases

2) Extract & run (Linux / macOS)

   ```sh
   tar -xzf progressdb_0.1.0_linux_amd64.tar.gz
   chmod +x progressdb
   ./progressdb --db ./data
   ```

3) Windows

   Unzip and run `progressdb.exe` from PowerShell.

That’s it — download, run, and connect with the SDKs below. Have fun!


## Install SDKs

- Python backend SDK:

  ```sh
  pip install progressdb
  ```

- Node backend SDK:

  ```sh
  npm install @progressdb/node
  ```

- Frontend SDKs (TypeScript / React):

  ```sh
  npm install @progressdb/js @progressdb/react
  ```

## Quick examples

Python (backend):

```py
from progressdb import ProgressDBClient

client = ProgressDBClient(base_url='http://localhost:8080', api_key='ADMIN_KEY')

# Sign a user id (backend-only)
sig = client.sign_user('user-123')

# Create a thread (provide author)
thread = client.create_thread({'title': 'General'}, author='service-account')

# Create a message (provide author)
msg = client.create_message({'thread': thread['id'], 'body': {'text': 'hello'}}, author='service-account')
```

Node (backend):

```js
import ProgressDB from '@progressdb/node'

const client = new ProgressDB({ baseUrl: 'http://localhost:8080', apiKey: process.env.PROGRESSDB_ADMIN_KEY })

await client.signUser('user-123')
const thread = await client.createThread({ title: 'General' }, 'service-account')
await client.createMessage({ thread: thread.id, body: { text: 'hello' } }, 'service-account')
```



JavaScript (frontend)

```js
import ProgressDBClient from '@progressdb/js'

const client = new ProgressDBClient({ baseUrl: 'http://localhost:8080', apiKey: 'pk_frontend' })

// list threads (frontend: requires signed user; set `defaultUserId`/`defaultUserSignature`)
const { threads } = await client.listThreads()

// create a thread
const thread = await client.createThread({ title: 'General' })

// post a message to a thread
const msg = await client.createThreadMessage(thread.id, { body: { text: 'Hello from the web!' } })

console.log('Posted message', msg)
```

React (frontend)

```jsx
import React from 'react'
import { ProgressDBProvider, useMessages } from '@progressdb/react'

function Chat({ threadId }) {
  const { messages, loading, refresh, create } = useMessages(threadId)

  if (loading) return <div>Loading…</div>
  return (
    <div>
      <ul>
        {messages?.map(m => (
          <li key={m.id}>{m.body?.text || JSON.stringify(m.body)}</li>
        ))}
      </ul>
      <button onClick={() => create({ body: { text: 'Hi from React!' } })}>Send</button>
    </div>
  )
}

export default function App() {
  return (
    <ProgressDBProvider
      options={{ baseUrl: 'http://localhost:8080', apiKey: 'pk_frontend' }}
      getUserSignature={async () => ({ userId: 'user-123', signature: 'sig-placeholder' })}
    >
      <Chat threadId="general" />
    </ProgressDBProvider>
  )
}
```

## Features

Implemented / Available
- [x] Messages - append-only storage, versioning (edits), replies, soft-delete
- [x] Threads - metadata operations (create/update/list)
- [x] Encryption & Key Management - (embedded KMS mode)
- [x] Retention - policy-driven purge/run hooks
- [x] Backend SDKs - node & python sdks published for v0.1.0
- [x] Frontend SDKs - typescript & react sdks published for v0.1.0

Partially implemented / Experimental
- [ ] Metrics - Prometheus metrics (hooks present - metrics coverage is incomplete)
- [ ] Reliability - (WAL/buffering) are present in parts but not fully hardened for production
- [ ] Performance - performance benchmarking tools are available - but SLO/alerting integrations are incomplete

Planned / Not implemented
- [ ] Encryption - Cloud-backed KMS / HSM integration
- [ ] Backups - backups & tested restore workflows
- [ ] Realtime - realtime subscriptions (WebSocket / SSE) and webhook delivery
- [ ] Search - search API / indexed search experience
- [ ] Scaling - horizontal scaling features (sharding, partitioning, cluster replication)

[![test-db-service](https://github.com/ha-sante/ProgressDB/actions/workflows/test-db-service.yml/badge.svg)](https://github.com/ha-sante/ProgressDB/actions/workflows/test-db-service.yml)
