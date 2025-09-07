SDK build & publish scripts (moved to `/.scripts/sdk`)

- `build-js-sdk.sh` — builds the TypeScript SDK into `clients/sdk/frontend/typescript/dist` using `npm run build`.
- `publish-js-npm.sh` — publishes the `dist` folder to npm (requires proper npm login and permissions).
- `publish-js-jsr.sh` — publishes to the JSR (Deno) registry via `npx jsr publish` (interactive auth). Ensure `mod.ts` exists and `package.json` contains `exports`.

