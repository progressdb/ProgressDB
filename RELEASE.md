## Release Guide

This document explains how to publish the different artifacts produced from this repository. The sections below are organized by artifact type so you can follow the local/manual steps (useful for testing) or the automated CI flow (GitHub Actions + goreleaser).

Tips:
- Use Git tags of the form `vMAJOR.MINOR.PATCH` (for example `v0.1.0`) as the canonical release versions. Tags trigger the CI release workflow.
- Avoid `sudo` when building locally — files in `dist/` will become root-owned and make CI uploads fail.

---

## 1) Server binaries (compiled Go executables)

What we produce
- Multi-platform single-file binaries for Linux, macOS and Windows plus archives (tar.gz / zip) and checksums.

Local/manual flow (quick):
1. Build a single binary (example):

   ```sh
   VERSION=0.1.0 COMMIT=$(git rev-parse --short HEAD) BUILDDATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
     CGO_ENABLED=0 GOOS=linux GOARCH=amd64 scripts/build.sh OUT=dist/progressdb-linux-amd64
   ```

2. Or run the bundled multi-platform release script:

   ```sh
   chmod +x scripts/release.sh
   ./scripts/release.sh v0.1.0
   ```

3. Inspect `dist/` for archives and checksums.

CI flow (recommended):
- Push a Git tag: `git tag v0.1.0 && git push origin v0.1.0`.
- GitHub Actions runs `.github/workflows/release.yml`, which invokes `goreleaser` to build all targets, create archives, checksums, and publish a GitHub Release with artifacts attached.

Notes & secrets:
- The basic goreleaser workflow only needs `GITHUB_TOKEN` (automatically provided by Actions).
- If you want to GPG-sign artifacts, add `GPG_PRIVATE_KEY` and `GPG_PASSPHRASE` to repository secrets and update `.goreleaser.yml` to enable signing.

---

## 2) Docker images (optional)

What we produce
- Docker images that run the compiled binary.

Local/manual flow:
1. Build the binary as above and create a Dockerfile (example in `RELEASE.md` earlier).
2. Build and tag locally:

   ```sh
   docker build -t yourorg/progressdb:0.1.0 .
   docker push yourorg/progressdb:0.1.0
   ```

CI flow with goreleaser (recommended):
- goreleaser can build and push Docker images as part of the release. Add `DOCKER_USERNAME`/`DOCKER_PASSWORD` (or `CR_PAT`) to GitHub Secrets and enable the `docker` block in `.goreleaser.yml`.

---

## 3) Backend SDKs

Python SDK (clients/sdk/backend/python)
- Local/manual publish:
  ```sh
  cd clients/sdk/backend/python
  python3 -m pip install --upgrade build twine
  python3 -m build
  python3 -m twine upload dist/*
  ```
- Helper script (interactive):
  ```sh
  ./.scripts/sdk/publish-python.sh --yes
  ```
- CI publish:
  - Add `PYPI_API_TOKEN` to repository secrets (use `__token__` as the username for `twine` uploads).
  - Add a CI job or workflow step that runs `python3 -m build` and uploads with twine using `PYPI_API_TOKEN`.

Node SDK (clients/sdk/backend/nodejs)
- Local/manual publish:
  ```sh
  cd clients/sdk/backend/nodejs
  npm publish --access public
  ```
- Helper scripts:
  - `.scripts/sdk/publish-node.sh` (interactive helper for npm/JSR publishes)

Notes:
- The Python package name is `progressdb` as set in `pyproject.toml`.
- For CI publishes, set `NPM_TOKEN` for npm and `PYPI_API_TOKEN` for PyPI.

---

## 4) Frontend SDKs (TypeScript, React)

Local/manual publish:
- `clients/sdk/frontend/typescript` (package `@progressdb/js`)
- `clients/sdk/frontend/reactjs` (package `@progressdb/react`)

Example:
```sh
cd clients/sdk/frontend/reactjs
npm publish --access public
```

Helper scripts:
- `.scripts/sdk/publish-react.sh` — interactive helper that builds and publishes the React package (and can publish to JSR then npm).

CI publish:
- Add `NPM_TOKEN` as a repository secret and use a workflow step to run `npm publish`.

---

## 5) Docs & OpenAPI

- The OpenAPI spec is at `./docs/openapi.yaml` and the swagger UI is served at `/docs` by the running server.
- The admin viewer is the static site under `./viewer` and served at `/viewer/` by the server.

Publishing docs:
- You can publish the OpenAPI and docs to a static site (GitHub Pages or similar). Optionally add a CI job that pushes built docs to `gh-pages`.

---

## 6) GitHub Release

- The canonical release is the GitHub Release created by goreleaser on tag push. Artifacts (archives, checksums, binaries) are attached automatically.
- Keep a `CHANGELOG.md` and reference it in release notes; goreleaser can include changelog snippets if configured.

---

## Checklist for a full release

- [ ] Update `CHANGELOG.md` and commit changes.
- [ ] Bump any SDK versions if needed (npm/pyproject updates).
- [ ] Tag the release: `git tag vX.Y.Z && git push origin vX.Y.Z`.
- [ ] Confirm GitHub Actions has `GITHUB_TOKEN` (automatic) and any optional secrets you need (NPM_TOKEN, PYPI_API_TOKEN, DOCKER credentials, GPG keys).
- [ ] Verify that the goreleaser workflow runs and a GitHub Release is created with assets.

---

If you want, I can now:
- add GPG signing and Docker publishing sections to `.goreleaser.yml`, or
- add CI jobs to publish PyPI/npm directly after goreleaser completes. 
Which would you like next?
