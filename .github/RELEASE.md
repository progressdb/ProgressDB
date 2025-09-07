# Release Automation with GitHub Actions & GoReleaser

This repository uses [GoReleaser](https://goreleaser.com/) (see `.goreleaser.yml`) and GitHub Actions (see `.github/workflows/release.yml`) to automate multi-platform builds, release archives, checksums, and Docker images. When you push a tag like `v1.2.3`, the workflow runs and publishes everything for you.

---

## What Happens on Release?

- **Builds**: Cross-compiles binaries for all configured OS/ARCH targets.
- **Archives**: Packages binaries as `.tar.gz` (Unix) or `.zip` (Windows), and generates checksums.
- **Publishes**:
  - Uploads all artifacts to a new GitHub Release.
  - Optionally pushes Docker images to Docker Hub and GitHub Container Registry (GHCR).
  - Can also publish to Homebrew, npm, PyPI, S3, etc. (if configured).

---

## Required Secrets

- **`GITHUB_TOKEN`**: Provided automatically by GitHub Actions. Used for GitHub Releases and GHCR. No setup needed.
- **`DOCKERHUB_USERNAME` / `DOCKERHUB_PASSWORD`**: Needed if you want to publish Docker images to Docker Hub. Add these as repository secrets.

### Optional (for advanced publishing):

- `GPG_PRIVATE_KEY` / `GPG_PASSPHRASE`: For signing archives/checksums.
- `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` / `AWS_REGION`: For S3 uploads.
- `NPM_TOKEN`: For npm package publishing.
- `PYPI_API_TOKEN`: For PyPI publishing (use as password, username `__token__`).
- `HOMEBREW_GITHUB_TOKEN` / `HOMEBREW_REPO`: For Homebrew tap publishing.

---

## How to Add Secrets

1. Go to your GitHub repo.
2. Click **Settings** → **Secrets and variables** → **Actions**.
3. Click **New repository secret**, enter the name and value, and save.

---

## Workflow Permissions

Make sure your workflow has the right permissions (already set in our repo):
