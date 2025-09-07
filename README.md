
![ProgressDB Logo](/docs/logo-colors.png)

ProgressDB is a fast, purpose-built, chat-native database for AI chat threads. The project includes a database service, backend SDKs (Node, Python), and frontend SDKs (TypeScript, React). This quickstart shows how to run the service locally, install the SDKs, and perform basic operations.

## Why ProgressDB?

ProgressDB is built specifically for chat threads and makes common chat workflows simple and fast:

- Start a thread by sending a message — the database will auto-create the thread for you.
- Built-in message versioning, edits, replies, reactions, and soft-deletes.
- Optimized for fast threaded message retrievals and common chat patterns.
- Straightforward encryption and API-key based access controls.
- Ship quickly: small service, simple APIs, and SDKs for Python, Node and frontend use.

ProgressDB removes friction when building chat-first apps: fewer transformation layers, direct APIs for threads/messages, and tooling to get you from prototype to production faster.


## Quickstart — Run the service (download a release)

Want to try ProgressDB in seconds? Download a prebuilt release — no Go toolchain required.

1. Visit Releases and pick the binary for your platform:

   https://github.com/ha-sante/ProgressDB/releases

2. Extract and run (example for Linux/macOS):

   ```sh
   tar -xzf progressdb_0.1.0_linux_amd64.tar.gz
   chmod +x progressdb
   ./progressdb --db ./data
   ```

3. On Windows, unzip and run `progressdb.exe`.

When the service starts it prints a friendly banner. After startup:

- Open the admin viewer: `http://localhost:8080/viewer/`
- Open the API docs: `http://localhost:8080/docs/`
- Check metrics: `http://localhost:8080/metrics`

That’s it — download, run, and you’re ready to connect with the SDKs below. It’s fast and fun: try sending a message and watch ProgressDB auto-create a thread for you.

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

## Features

Implemented (ready to use):

- [x] Message storage (append-only), versions, soft-delete, replies, reactions
- [x] Thread metadata (CRUD)
- [x] Structured logging, Prometheus metrics, config & security middleware
- [x] Backend SDKs (Node, Python)
- [x] Frontend SDKs (TypeScript, React)
- [x] Simple data viewer

Planned / In progress:

- [ ] Performance benchmarking & SLO/alerts
- [ ] Backups & tested restores
- [ ] Encryption key management & rotation
- [ ] Realtime subscriptions (WebSocket/SSE) & webhook delivery
- [ ] API versioning, retention policies, scaling and search

## Links

- OpenAPI spec: `docs/openapi.yaml` (served at `/docs/` when the service is running)
- Admin viewer: `viewer/` (served at `/viewer/` when the service is running)
- Metrics endpoint: `/metrics` (Prometheus)
- Backend SDKs: `clients/sdk/backend`
- Frontend SDKs: `clients/sdk/frontend`
- Releases (download binaries): https://github.com/ha-sante/ProgressDB/releases
- Contribution guide: `CONTRIBUTING.md`

## How to start contributing

1. Read `CONTRIBUTING.md` for the development workflow and PR checklist.
2. Fork the repo and create a branch for your change (e.g. `feat/your-feature`).
3. Run tests and linters locally, add tests for new behavior.
4. Open a pull request against `main` with a clear description and testing notes.

For development builds and advanced builds, see the "Build from source (advanced)" section above.
