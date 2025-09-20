SDK build & publish scripts (moved to `/.scripts/sdk`)

Available scripts:

- `publish-js.sh` — interactive single-file publisher for the JSR (Deno) registry. Prompts for build and runs `npx jsr publish`.
- `publish-node.sh` — interactive single-file publisher for npm. Prompts to bump the package version, builds, and publishes to npm. Supports `--dry-run` to inspect packing without publishing.
- `publish-python.sh` — interactive publisher for the Python backend SDK. Changes to the Python package directory, builds a wheel/sdist and uploads to PyPI via `twine`. Supports `--dry-run` to only build artifacts and `--yes` to skip prompts.

Examples:

  # Interactive JSR publish
  ./.scripts/sdk/publish-js.sh

  # Interactive npm publish with automatic bump and build
  ./.scripts/sdk/publish-node.sh

React package publishing:

- `./.scripts/sdk/publish-react.sh` — interactive publisher for the React package (`@progressdb/react`). Publishes to JSR first, then optionally npm. Options: `--yes`, `--no-build`, `--dry-run`, `--allow-slow-types`.

Example:

  ./.scripts/sdk/publish-react.sh --yes

Python package publishing:

- Build and publish the backend Python package (located at `clients/sdk/backend/python`):

  ./.scripts/sdk/publish-python.sh --dry-run

Notes:
- `publish-python.sh` runs in `clients/sdk/backend/python`, builds wheel and sdist with `python3 -m build`, and uploads using `twine upload` unless `--dry-run` is supplied.

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

Test helpers

- `./scripts/sdk/test-node.sh` — run Node SDK tests (backend). Usage: `./scripts/sdk/test-node.sh [--unit|--integration|--all] [--watch]`.
- `./scripts/sdk/test-frontend.sh` — run frontend SDK tests (typescript + react). Usage: `./scripts/sdk/test-frontend.sh [--unit|--integration|--all] [--watch]`.
- `./scripts/sdk/test-all-sdks.sh` — run all SDK tests sequentially (or `--watch` to run both in watch mode).

Examples:

  # Run backend unit tests
  ./scripts/sdk/test-node.sh --unit

  # Run all frontend tests in watch mode
  ./scripts/sdk/test-frontend.sh --all --watch
