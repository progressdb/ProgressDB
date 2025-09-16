# ProgressDB Development Roadmap

**Service Development**

- [x]  Logging
- [x]  Metrics
- [ ]  Performance - *(measure & alert for low speeds & public banners)*
- [x]  Documentation - *(api & general docs)*
- [x]  Testing Suite & Tests - *(api & utils)*
- [x]  Config Flags - *(inline flags etc)*
- [x]  Security *- (cors, apikeys, tls etc)*
- [x]  Rate limiting
- [x]  Messages - *(edits ~ versioning, deletes, replies, reacts)*
- [x]  Threads - *(crud a thread, relationship with messages & effects - base at most, id and names for threads)*
- [x]  Auth - *(authenticated authors & accessible datas)*
- [ ]  Updates - *(api versioning & image or instance upgrades)*
- [ ]  Sockets - *(realtime subscriptions, webhooks)*
- [x]  Encryption - *(local kms encryption)*
- [ ]  State Changes - *(shutdowns, health, restarts - integrity checks etc)*
- [ ]  Backups - *(cloud backups)*
- [ ]  Retention - *(data retention policy)*
- [ ]  Encryption - *(cloud backed kms~hsm encryption)*

**Backend SDKs**

- [x]  Nodejs SDK
- [x]  Python SDK

**Frontend SDKs**

- [x]  Typescript SDK - *(provides the direct methods to e.g fetch threads etc)*
- [x]  React SDK - *(provides easy hooks e.g ThreadInfo, ThreadMessages)*

**User Products**

- [x]  Docker Image
- [x]  Binaries
- [ ]  Github registry image

**System Upgrades**

- [ ]  Scaling - *(clustering, performance tests etc)*
- [ ]  Realtime - *(websockets & subscribers + client cache)*
- [ ]  Search - *(per author messages or threads search)*
- [ ]  Sounds - *(sent, received message sounds)*

**Experience Upgrades**

- [ ]  AI tooling - *(thread summary & event callbacks etc)*
- [ ]  Third Integrations - *(vercelai, chromadb, langchain etc)*
- [ ]  Start Templates - *(nextjs, react, vue)*
- [ ]  Replies - *(add sdk helpers)*
- [ ]  React - *(add sdk helpers)*

**Developer Tools**

- [x]  Data Viewer - *(easy data viewer using admin keys)*
- [ ]  Debug Mode - *(logs everything)*
- [ ]  CLI - *(easy analytics & status inspection)*
- [ ]  ProgressCloud - *(managed hosting service)*
