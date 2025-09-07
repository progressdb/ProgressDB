# Release Guide

This document explains how to publish the pieces of this repository so users can consume them: server binaries, Docker images, backend SDKs (Python, Node), frontend SDKs, and GitHub Releases. It describes both local/manual steps and the repository CI flow.

## Release Types

- **Server binaries:** single-file compiled Go executables (multi-arch) distributed in tar/zip archives.
- **Docker images:** optional container images for running the server.
- **Backend SDKs:** Python package published to PyPI (`progressdb`) and Node package published to npm (`@progressdb/node`).
- **Frontend SDKs:** `@progressdb/js` and `@progressdb/react` published to npm.
- **Docs & OpenAPI:** `./docs/openapi.yaml` and viewer served from `/docs` and `/viewer`.
- **GitHub Release:** canonical release entry that attaches binary artifacts, checksums, and release notes.

## Versioning

- Use Git tags in the form `vMAJOR.MINOR.PATCH` (e.g. `v0.1.0`). Tags drive the CI release workflow.
- The build system injects version metadata into the binary via ldflags (`VERSION`, `COMMIT`, `BUILDDATE`).

## Quick local release (binaries)

1. Make sure your working tree is clean and commit everything you need.
2. Tag the release:

   ```sh
   git tag v1.2.3
   git push origin v1.2.3
   ```

3. Locally build and package all platforms with the helper script:

   ```sh
   chmod +x scripts/release.sh
   ./scripts/release.sh v1.2.3
   ```

   - Artifacts and archives will be in `./dist/`.
   - The script injects `VERSION`, `COMMIT` and `BUILDDATE` into the binaries.

4. Inspect the artifacts and checksums in `./dist/`.

5. Create a GitHub Release and upload artifacts, or push the tag and use CI to create the release automatically.

## CI releases (GitHub Actions)

- The workflow `.github/workflows/release.yml` runs when a tag `v*.*.*` is pushed.
- Behavior:
  - Builds a matrix of OS/ARCH targets using `scripts/build.sh`.
  - Packages and uploads artifacts as a workflow artifact.
  - Creates a GitHub Release and attaches the packaged artifacts.

### What the workflow needs

- `GITHUB_TOKEN` — provided automatically by GitHub Actions for the repo; no manual setup required.
- Optional secrets for extra features (only add if you plan to use them):
  - `GPG_PRIVATE_KEY`, `GPG_PASSPHRASE` — sign artifacts.
  - `DOCKER_USERNAME`, `DOCKER_PASSWORD` (or `CR_PAT`) — push Docker images.
  - `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION` — upload to S3.
  - `NPM_TOKEN` — publish npm packages.
  - `PYPI_API_TOKEN` — publish to PyPI (use `__token__` user for `twine`).

Add secrets at: `Repository` -> `Settings` -> `Secrets and variables` -> `Actions` -> `New repository secret`.

## Publish Python SDK (PyPI)

Local publish flow (manual credentials):

1. From `clients/sdk/backend/python` run:

   ```sh
   python3 -m pip install --upgrade build twine
   python3 -m build
   python3 -m twine upload dist/*
   ```

2. Use the helper script (interactive):

   ```sh
   ./.scripts/sdk/publish-python.sh --yes
   ```

CI publish flow:

- Set `PYPI_API_TOKEN` in Actions secrets.
- Add a workflow step (or new workflow) that runs the build and uses `twine` to upload with `PYPI_API_TOKEN`.

Notes:
- Ensure package name in `clients/sdk/backend/python/pyproject.toml` is correct (`progressdb`).
- Verify PyPI namespace availability before publishing.

## Publish Node / Frontend SDKs (npm)

Local publish (interactive):

1. For the Node backend package (`clients/sdk/backend/nodejs`):

   ```sh
   cd clients/sdk/backend/nodejs
   npm publish --access public
   ```

2. For frontend packages (`clients/sdk/frontend/*`):

   ```sh
   cd clients/sdk/frontend/reactjs
   npm publish --access public
   ```

CI publish flow:

- Add `NPM_TOKEN` as a repository secret.
- Use a CI step that sets `NPM_TOKEN` in `~/.npmrc` (or uses `npm publish --//registry.npmjs.org/:_authToken=$NPM_TOKEN`).

Notes:
- The repo contains helper scripts in `.scripts/sdk` (e.g. `publish-node.sh`, `publish-react.sh`) to standardize publishes.

## Docker images

- Create a Dockerfile in the repo root or `server/` (example below). Build and push via CI.

Example Dockerfile:

```dockerfile
FROM debian:bookworm-slim
COPY ./dist/progressdb /usr/local/bin/progressdb
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/progressdb"]
```

CI steps:
- Build the binary + Docker image, tag as `org/progressdb:1.2.3`, and push to registry using Docker credentials (`DOCKER_USERNAME`/`DOCKER_PASSWORD` or `CR_PAT`).

## Checksums & Signing

- The `scripts/release.sh` generates `.tar.gz` / `.zip` archives and `.sha256` checksums.
- Optionally sign artifacts with GPG and publish signatures alongside archives. Add `GPG_PRIVATE_KEY`/`GPG_PASSPHRASE` to Actions secrets if you plan to sign in CI.

## Changelogs & Release Notes

- Keep release notes or a changelog in `CHANGELOG.md` and reference it when creating GitHub releases.
- For automated changelogs, consider `github_changelog_generator` or `goreleaser` changelog hooks.

## Troubleshooting

- If tar fails with `Cannot stat: No such file or directory`, check that the build produced the expected file name in `dist/` and that `OUT` env var is respected.
- Avoid running build scripts with `sudo` — files in `dist/` will become root-owned and CI may not be able to upload them.
- If a GitHub Action cannot create a release, ensure `GITHUB_TOKEN` is available and the workflow has permission to `contents: write` (repo settings).

## Optional: Use goreleaser

- `goreleaser` can simplify multi-platform builds, archives, checksums, GPG signing, and GitHub Release publishing.
- Install and add a `.goreleaser.yml` if you want a more feature-rich release flow.

## Summary checklist

- [ ] Tag the release `git tag vX.Y.Z && git push origin vX.Y.Z`
- [ ] Ensure `dist/` artifacts are produced locally or CI will build them
- [ ] Confirm required secrets exist in GitHub Actions (see above)
- [ ] Create a GitHub Release or let CI create it automatically
- [ ] Publish SDK packages (PyPI / npm) if their versions should be released in lockstep

---
If you want, I can:
- add a `--version` CLI flag to the server and a `/version` HTTP endpoint,
- add automatic PyPI/npm publish steps to the CI workflow,
- or create a `goreleaser` config and replace the custom workflow with goreleaser usage.

