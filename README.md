
![ProgressDB Logo](/docs/logo-colors.png)

ProgressDB is a fast, purpose-built, chat-native database for AI chat threads. The project includes a small server, backend SDKs (Node, Python), and frontend SDKs (TypeScript, React). This quickstart shows how to run the server locally, install the SDKs, and perform basic operations.

## Quickstart — Run locally

- **Prerequisites:** `go` (1.20+), `python3` (3.8+), `node` & `npm` (or `pnpm`/`yarn`).
- **Dev (fast):** start the server from the repo root (uses local modules):

  ```sh
  ./.scripts/dev.sh
  # or equivalently
  go run ./server/cmd/progressdb
  ```

- **Build binary:**

  ```sh
  ./.scripts/build.sh
  # binary written to ./dist/progressdb by default
  ```

The server serves the admin viewer at `http://localhost:8080/viewer/`, the OpenAPI UI at `/docs/` and Prometheus metrics at `/metrics`.

## Install SDKs

- Python backend SDK:

  ```sh
  pip install progressdb
  ```

- Node backend SDK:

  ```sh
  npm install @progressdb/node
  # or: pnpm add @progressdb/node
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

# Create a thread
thread = client.create_thread({'title': 'General'})

# Create a message
msg = client.create_message({'thread': thread['id'], 'body': {'text': 'hello'}})
```

Node (backend):

```js
import ProgressDB from '@progressdb/node'

const client = new ProgressDB({ baseUrl: 'http://localhost:8080', apiKey: process.env.PROGRESSDB_ADMIN_KEY })

await client.signUser('user-123')
const thread = await client.createThread({ title: 'General' })
await client.createMessage({ thread: thread.id, body: { text: 'hello' } })
```

React (frontend) — use `@progressdb/react` hooks in your app to read threads/messages and render UI components.

## Where to look next

- API reference and OpenAPI: `http://localhost:8080/docs/` (served from `./docs/openapi.yaml`)
- Admin viewer: `http://localhost:8080/viewer/`
- Metrics: `http://localhost:8080/metrics`
- SDKs: `clients/sdk/backend` and `clients/sdk/frontend` directories for source and samples.

## What’s implemented (high level)

- Message storage (append-only), versions, soft-delete, replies, reactions
- Thread metadata (CRUD)
- Structured logging, Prometheus metrics, config & security middleware
- Backend SDKs (Node, Python), Frontend SDKs (TypeScript, React)
- Simple data viewer

## Remaining work (high level)

- Performance benchmarking & SLO/alerts
- Backups & tested restores
- Encryption key management & rotation
- Realtime subscriptions (WebSocket/SSE) & webhook delivery
- API versioning, retention policies, scaling and search

For more details see `clients/`, `server/` and `docs/` in the repo.
