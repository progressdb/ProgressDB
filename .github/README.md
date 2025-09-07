GitHub Actions: release workflow and required secrets
===============================================

This repository includes an automated release workflow at `.github/workflows/release.yml` that:

- Builds multi-platform Go binaries when you push a tag like `v1.2.3`.
- Packages the built binaries (tar.gz / zip) and uploads them as assets to a GitHub Release.

Required secrets and notes
--------------------------

- `GITHUB_TOKEN` (automatic):
  - GitHub Actions provides `GITHUB_TOKEN` automatically to workflows. You do not need to add this manually.
  - It's used by the release action to create the release and upload assets.

- Optional CI secrets you may want to add depending on extra automation you enable:
  - `GPG_PRIVATE_KEY` / `GPG_PASSPHRASE`: if you sign binaries or archives with GPG.
  - `DOCKER_USERNAME` / `DOCKER_PASSWORD` (or `CR_PAT`) : if you publish Docker images to Docker Hub or GitHub Container Registry.
  - `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` / `AWS_REGION` : if you upload artifacts to S3 during the pipeline.
  - `NPM_TOKEN`: if you add npm publishing to CI for frontend SDKs.
  - `PYPI_API_TOKEN`: if you add PyPI publishing for the Python SDK. Use `__token__` as username and the token as password when using `twine`.
  - `SLACK_WEBHOOK`: if you intend to post release notifications to Slack.

How to add secrets
-------------------

1. Go to your GitHub repository page.
2. Click `Settings` → `Secrets and variables` → `Actions`.
3. Click `New repository secret`, enter the name and value, and save.

Notes & recommendations
-----------------------
- Tags control the release: create a tag like `v1.2.3` and push it. The workflow runs on `push` to tag `v*.*.*`.
- The current workflow builds for a matrix of OS/ARCH targets and uploads all `dist/**` files to the release using `softprops/action-gh-release`.
- If you want more sophisticated releases (homebrew formula, Docker images, OS packages, GPG signing), consider using `goreleaser` and adding a `GORELEASER_KEY`/`GPG` secret.

Questions? Need me to add publish steps for PyPI / npm / Docker? I can add secure, token-based publish steps to the workflow and document required secrets.

