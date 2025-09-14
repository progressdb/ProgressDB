 # ProgressDB Development Roadmap

## Server Development

- [x] Logging
- [x] Metrics
- [x] Documentation - *(api & general docs)*
- [x] Testing Suite & Tests - *(api & utils)*
- [x] Config Flags - *(inline flags etc)*
- [x] Security *- (cors, apikeys, tls etc)*
- [x] Rate limiting
- [x] Messages - *(edits ~ versioning, deletes, replies, reacts)*
- [x] Threads - *(crud a thread, relationship with messages & effects)*
- [x] Auth - *(authenticated authors & accessible datas)*
- [ ] Performance - *(measure & alert for low speeds & public banners)*
- [ ] Backups - *(cloud backups)*
- [x] Encryption - *(data encryption - local easy kms)*
- [ ] State Changes - *(shutdowns, health, restarts)*
- [ ] Sockets - *(realtime subscriptions, webhooks)*
- [ ] Updates - *(api versioning & image or instance upgrades)*
- [ ] Retention - *(data retention policy)*
- [ ] Encryption - *(data encryption - cloud kms~hsm backed)*

## Backend SDKs

- [x] Nodejs SDK
- [x] Python SDK

## Frontend SDKs

- [x] Typescript SDK - *(provides the direct methods to e.g fetch threads etc)*
- [x] React SDK - *(provides easy hooks e.g ThreadInfo, ThreadMessages)*

## System Upgrades

- [ ] Scaling - *(clustering, performance tests etc)*
- [ ] Realtime - *(websockets & subscribers + client cache)*
- [ ] Search - *(per author messages or threads search)*
- [ ] Sounds - *(sent, received message sounds)*

## Developer Tools

- [x] Data Viewer - *(easy data viewer using admin keys)*
- [ ] Debug Mode - *(logs everything)*
- [ ] CLI - *(easy analytics & status inspection)*
- [ ] ProgressCloud - *(managed hosting service)*
