SDK build & publish scripts (moved to `/.scripts/sdk`)

Available scripts:

- `publish-js.sh` — interactive single-file publisher for the JSR (Deno) registry. Prompts for build and runs `npx jsr publish`.
- `publish-node.sh` — interactive single-file publisher for npm. Prompts to bump the package version, builds, and publishes to npm. Supports `--dry-run` to inspect packing without publishing.
- `build-node-sdk.sh` — (legacy) helper for building the Node backend SDK; left in place for backwards compatibility.

Examples:

  # Interactive JSR publish
  ./.scripts/sdk/publish-js.sh

  # Interactive npm publish with automatic bump and build
  ./.scripts/sdk/publish-node.sh

Notes:
- The scripts try to be safe: they check for `dist/`, require `npm` and `npx` where relevant, and will prompt or exit if not logged in for npm.
- Avoid running these as `sudo` unless you understand the implications; the build script warns if run as root.

New convenience scripts (both publish to JSR first, then npm):

- `publish-js.sh` — interactive single-file publisher that runs JSR publish then optionally publishes to npm. Prompts for build, and npm options (bump/dry-run). Options: `--yes`, `--no-build`, `--allow-slow-types`.
- `publish-node.sh` — interactive single-file publisher that builds, publishes to JSR, then publishes to npm. Prompts to bump the package version, builds, and publishes. Options: `--yes`, `--no-build`, `--dry-run`, `--allow-slow-types`.

Examples:

  # Interactive publish to both registries (JSR then npm)
  ./.scripts/sdk/publish-js.sh

  # Non-interactive publish to both registries
  ./.scripts/sdk/publish-node.sh --yes
