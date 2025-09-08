# Contributing

Thanks for your interest in contributing to ProgressDB — we appreciate it! This document explains the preferred workflow, coding conventions, and how to run tests and builds locally.

1) File issues
----------------
- For bugs: open an issue with a clear title, reproduction steps, expected vs actual behavior, and any relevant logs or configs.
- For feature requests: describe the problem you're trying to solve and a suggested approach or API.
- For security issues: do NOT open a public issue — see `SECURITY.md` for private reporting instructions.

2) Branches & commits
-----------------------
- Keep commits focused and atomic; use conventional commit messages where helpful (e.g., `feat:`, `fix:`, `chore:`).

3) Development workflow (pull requests)
--------------------------------------
- Fork the repo (if you don't have push access) and create a topic branch.
- Make your changes and add tests where appropriate.
- Run the repository tests and linters locally.
- Open a pull request against `main` with a clear description and testing notes.
- PR checklist (suggested):
  - The change is covered by tests where applicable.
  - All existing tests pass locally.
  - Documentation (README, CHANGELOG) updated if behavior or public API changed.
  - No secrets or credentials were added.

4) Coding conventions
----------------------
- Go server code: follow standard `gofmt`/`go vet` rules. Keep functions small and tests focused.
- JavaScript/TypeScript: follow repository ESLint/tsconfig patterns if present; format with `prettier` if configured.
- Python: keep code readable, follow PEP8, and use type hints where sensible.

5) Running locally
-------------------
- Start a dev server (fast, uses local modules):

  ```sh
  ./scripts/dev.sh
  # or: go run ./server/cmd/progressdb
  ```

- Build a single binary:

  ```sh
  scripts/build.sh
  # output: ./dist/progressdb
  ```

- Run tests (repo includes a helper):

  ```sh
  ./scripts/test.sh
  ```

6) Building releases locally
----------------------------
- Create multi-platform artifacts locally for testing:

  ```sh
  chmod +x scripts/release.sh
  ./scripts/release.sh v0.1.0
  # artifacts in ./dist/
  ```

7) SDKs and publishing notes
----------------------------
- Python SDK (`clients/sdk/backend/python`): build with `python -m build` and publish with `twine` or use `./.scripts/sdk/publish-python.sh`.
- Node SDKs (`clients/sdk/backend/nodejs`, `clients/sdk/frontend/*`): use `npm publish --access public` or the helper scripts in `scripts/sdk`.
- For CI publishes, use repository secrets (`PYPI_API_TOKEN`, `NPM_TOKEN`) and follow the instructions in `RELEASE.md` and `.github/README.md`.

8) Documentation changes
------------------------
- Update README, docs in `./docs/`, and `CHANGELOG.md` for notable changes.
- Keep examples minimal and copy-paste friendly.

9) Code review & merging
-------------------------
- Maintainers will review PRs and may request changes.
- After approval, a maintainer will merge the PR and ensure CI passes.


![ProgressDB Animation](/animation.gif)
