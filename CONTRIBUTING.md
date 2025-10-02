# Contributing to ProgressDB

Thank you for your interest in contributing! This guide covers how to file issues, submit code, follow conventions, and run the project locally.

## 1. Filing Issues

- **Bugs:** Open an issue with a clear title, reproduction steps, expected vs. actual behavior, and any relevant logs or configs.
- **Feature Requests:** Describe the problem and your suggested solution or API.
- **Security:** *Do not* open public issues for security concerns. See `SECURITY.md` for private reporting instructions.

## 2. Branches & Commits

- Keep commits focused and atomic.
- Use conventional commit messages when possible (e.g., `feat:`, `fix:`, `chore:`).

## 3. Development Workflow (Pull Requests)

- Fork the repo (if you don't have push access) and create a topic branch.
- Make your changes and add tests as appropriate.
- Run all tests and linters locally.
- Open a pull request against `main` with a clear description and testing notes.

**PR Checklist:**
- [ ] Tests cover the change (where applicable)
- [ ] All tests pass locally
- [ ] Documentation (README, CHANGELOG) updated if behavior or public API changed
- [ ] No secrets or credentials added

## 4. Coding Conventions

- **Go:** Use `gofmt`/`go vet`. Keep functions small and tests focused.
- **JavaScript/TypeScript:** Follow repository ESLint/tsconfig rules; use `prettier` if configured.
- **Python:** Follow PEP8, keep code readable, and use type hints where sensible.

## 7. Documentation Changes

- Update `README.md`, docs in `./docs/`, and `CHANGELOG.md` for notable changes.
- Keep examples minimal and copy-paste friendly.

## 8. Code Review & Merging

- Maintainers will review PRs and may request changes.
- After approval, a maintainer will merge the PR and ensure CI passes.