<!-- START OF CONTRIBUTING TEMPLATE -->
# Contributing to Primus-SaFE

Thanks for your interest in contributing. Primus-SaFE is AMD's full-stack platform for stable, large-scale model training on AMD GPU clusters (Kubernetes + ROCm).

## Reporting Issues

Use [GitHub Issues](../../issues) to report bugs or request features. Include a clear description, reproduction steps, and your environment:

- OS / kernel version
- ROCm version (and GPU model, e.g. MI300X)
- Kubernetes version
- Go version (for `SaFE/`, `Scheduler-Plugins/`) or Python version (for `Bench/`, `Lens/`)
- Relevant logs, manifests, or job specs

## Pull Request Workflow

1. Fork the repository and create a branch from `main`:
   ```bash
   git checkout -b feature/short-description
   ```
2. Make your change. Add tests and update docs if behavior changes.
   - Go components: `gofmt` / `go test ./...`
   - Python components: `black` / `pytest`
   - Keep changes scoped — split large refactors into multiple PRs.
3. Open a PR against `main`. Describe *what* changed and *why*; link any related issue.
4. Ensure CI passes — at minimum: `unit-test`, `build`, component-specific workflows (`lens-build`, `web-ci`, `web-safe-docker-build`), and `block-sensitive-secrets`. Request review from the relevant [CODEOWNERS](CODEOWNERS).

By opening a PR, you agree your contribution is licensed under the terms in [LICENSE](../LICENSE).

## External Contributors

This repo is part of the AMD-AGI org. Non-AMD contributors need admin approval before being added as collaborators and must follow AMD's [open-source contribution guidelines](https://github.com/ROCm/ROCm/blob/develop/CONTRIBUTING.md).

For security issues, do **not** open a public issue — see [SECURITY.md](SECURITY.md).
<!-- END OF CONTRIBUTING TEMPLATE -->
