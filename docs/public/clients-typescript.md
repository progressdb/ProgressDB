---
section: clients
title: "TypeScript Reference"
order: 3
visibility: public
---

# TypeScript / JavaScript SDK

This page covers the TypeScript SDK for browser and server-side usage.

Install:

```bash
npm install @progressdb/js
```

Usage (browser):

```js
import ProgressDBClient from '@progressdb/js'
const client = new ProgressDBClient({ baseUrl: 'http://localhost:8080', apiKey: 'pk_frontend' })
```

Auth: frontends must obtain a user signature from a trusted backend and set
`X-User-ID` and `X-User-Signature` on requests to protected endpoints.

