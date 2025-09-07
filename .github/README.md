GitHub Actions: goreleaser + Docker publishing
============================================

This repository uses goreleaser to build binaries, archives, and publish Docker images to both GitHub Container Registry (GHCR) and Docker Hub as part of a release.

Required secrets
----------------
- `GITHUB_TOKEN` — provided automatically by GitHub Actions (used for GHCR auth in our workflow).
- `DOCKERHUB_USERNAME` and `DOCKERHUB_PASSWORD` — credentials for Docker Hub publishing.

Workflow permissions
--------------------
Ensure the release workflow has package permissions. Example (already configured):

```yaml
permissions:
  contents: read
  packages: write
  id-token: write
```

Notes
-----
- GHCR publishing uses the workflow actor + `GITHUB_TOKEN` (no extra secret required), but `packages: write` permission must be enabled.
- Docker Hub publishing requires `DOCKERHUB_USERNAME`/`DOCKERHUB_PASSWORD` to be added to repository secrets.

