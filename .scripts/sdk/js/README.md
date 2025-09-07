SDK build & publish scripts

- `build-js-sdk.sh` — builds the TypeScript SDK into `clients/sdk/frontend/typescript/dist` using `npm run build`.
- `publish-js-npm.sh` — publishes the `dist` folder to npm (requires proper npm login and permissions).
- `publish-js-jsr.sh` — placeholder to publish to a custom JS registry; set `REGISTRY_URL` before use.

