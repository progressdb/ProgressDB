GitHub Actions: goreleaser setup and required secrets
===================================================

This repository uses `goreleaser` (configured in `.goreleaser.yml`) to produce multi-platform binaries, archives, checksums, and GitHub Releases when a tag is pushed (tags like `v1.2.3`). The workflow is defined at `.github/workflows/release.yml` and invokes `goreleaser` on tag pushes.

What goreleaser does for us
---------------------------

- Builds binaries for configured OS/ARCH targets.
- Produces archives (`tar.gz` for unix, `zip` for windows) and checksums.
- Uploads artifacts to GitHub Releases and can optionally publish Docker images, Homebrew taps, and more.

Required secrets and notes
--------------------------

- `GITHUB_TOKEN` (automatic):
  - Provided by GitHub Actions automatically and used by goreleaser to create releases and upload assets. No manual setup required for the basic flow.

- Optional secrets you may want to add if you enable additional features:
  - `GPG_PRIVATE_KEY` / `GPG_PASSPHRASE`: sign archives and checksums.
  - `DOCKER_USERNAME` / `DOCKER_PASSWORD` (or `CR_PAT`): push Docker images to registries.
  - `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` / `AWS_REGION`: upload artifacts to S3.
  - `NPM_TOKEN`: publish frontend or backend JS packages to npm.
  - `PYPI_API_TOKEN`: publish Python packages to PyPI (use token as password with username `__token__`).
  - `HOMEBREW_GITHUB_TOKEN` / `HOMEBREW_REPO`: if you publish Homebrew taps.

How to add secrets
-------------------

1. Go to your GitHub repository page.
2. Click `Settings` → `Secrets and variables` → `Actions`.
3. Click `New repository secret`, enter the name and value, and save.

Local testing with goreleaser
----------------------------

- Install goreleaser locally (Homebrew or script):
  - `brew install goreleaser` or
  - `curl -sL https://goreleaser.com/install.sh | bash`.
- Test a snapshot release locally (won't publish to GitHub):
  - `goreleaser release --snapshot --rm-dist`

Notes & recommendations
-----------------------

- Tag the release: `git tag v1.2.3 && git push origin v1.2.3`. The workflow runs on tag pushes and goreleaser will publish a GitHub Release.
- If you require signing, Docker pushes, or publishing SDKs as part of a release, add the corresponding secrets and extend `.goreleaser.yml` with the relevant sections.
- Keep changelog entries in `CHANGELOG.md` and include release notes in GitHub Release; goreleaser can be configured to include changelog snippets automatically.

If you want, I can:
- add GPG signing to `.goreleaser.yml` and the workflow (requires adding `GPG_PRIVATE_KEY`/`GPG_PASSPHRASE`),
- add Docker publish steps via goreleaser (requires `DOCKER_*` secrets), or
- add PyPI / npm publishing integration in the release flow.
