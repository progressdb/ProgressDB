# ProgressDB Development Roadmap

## Server Development

- Unfinished / High Priority
  - [ ] Performance - *(measure & alert for low speeds & public banners)*
  - [ ] Backups - *(cloud backups)*
  - [ ] Encryption - *(backend encryption & key management)*
  - [ ] State Changes - *(shutdowns, health, restarts / graceful drain)*
  - [ ] Sockets - *(realtime subscriptions, webhooks)*
  - [ ] Updates - *(API versioning & instance upgrades)*

- Completed / Lower Priority
  - [x] Logging
  - [x] Metrics
  - [x] Documentation *(API & general docs)*
  - [x] Testing Suite *(API & utils)*
  - [x] Config Flags *(inline flags etc)*
  - [x] Security *(CORS, API keys, TLS etc)*
  - [x] Rate limiting
  - [x] Messages *(edits ~ versioning, deletes, replies, reacts)*
  - [x] Threads *(CRUD a thread, relationship with messages & effects - base at most, id and names for threads)*

## Backend SDKs

- Unfinished / High Priority
  - [x] Node.js SDK
  - [x] Python SDK

## Frontend SDKs

- Unfinished / High Priority
  - [x] Typescript core SDK
  - [x] React.js SDK *(Next.js support)*

## System Upgrades

- Unfinished / High Priority
  - [ ] Scaling *(clustering, performance tests etc)*
  - [ ] Realtime *(websockets & subscribers + client cache)*
  - [ ] Search
  - [ ] Sounds *(sent, received message sounds)*

## Developer Tools

- Unfinished / High Priority
  - [ ] Debug Mode *(logs everything)*
  - [ ] CLI *(easy analytics & status inspection)*
  - [ ] ProgressCloud
  - [ ] Website & Documentation section

- Completed / Lower Priority
  - [x] Data Viewer - *(easy data viewer using admin keys)*
