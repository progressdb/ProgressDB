SDK build & publish scripts (moved to `/.scripts/sdk`)

Available scripts:

- `build-js-sdk.sh` — builds the TypeScript SDK into `clients/sdk/frontend/typescript/dist` using `npm run build`. Usage: `./build-js-sdk.sh [--sdk-dir <path>] [--no-install]`.
- `publish-js-npm.sh` — publish helper. Usage: `./publish-js-npm.sh [--sdk-dir <path>] [--build-first] [--dry-run]`.
- `publish-js-jsr.sh` — publish helper for JSR/Deno registry. Usage: `./publish-js-jsr.sh [--sdk-dir <path>] [--build-first] [--allow-slow-types]`.
- `publish.sh` — user-friendly wrapper combining build + publish steps:
  - `./publish.sh build`
  - `./publish.sh publish-npm` (builds by default)
  - `./publish.sh publish-jsr` (builds by default)
  - `./publish.sh publish-all`

Examples:

  # Build only
  ./.scripts/sdk/build-js-sdk.sh

  # Dry-run npm publish
  ./.scripts/sdk/publish-js-npm.sh --build-first --dry-run

  # Full publish to both npm and jsr
  ./.scripts/sdk/publish.sh publish-all

Notes:
- The scripts try to be safe: they check for `dist/`, require `npm` and `npx` where relevant, and will prompt or exit if not logged in for npm.
- Avoid running these as `sudo` unless you understand the implications; the build script warns if run as root.
