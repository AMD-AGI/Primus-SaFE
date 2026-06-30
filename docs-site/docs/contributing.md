---
sidebar_position: 10
title: Contributing
---

# Contributing

Contributions to Primus-SaFE — code and docs — are welcome. The canonical guide (issues, PR
workflow, CI checks, CODEOWNERS, security policy) lives in
[`.github/CONTRIBUTING.md`](https://github.com/AMD-AGI/Primus-SaFE/blob/main/.github/CONTRIBUTING.md);
this page is a quick orientation.

<!-- @test none: meta/contributor page — not product behavior. -->

## Repository layout

| Module | What it is |
|--------|------------|
| `SaFE/` | The platform layer — apiserver, job manager, resource manager, webhooks (Go). |
| `Bootstrap/` | Provision Kubernetes and base add-ons; storage, gateway, registry scripts. |
| `Bench/` | Primus-Bench — node health checks and benchmarking (Python). |
| `Scheduler-Plugins/` | The topology-aware, gang-scheduling kube-scheduler (Go). |
| `Web/` | The browser console. |
| `docs-site/` | This documentation site (Docusaurus). |

## Code contributions

Fork, branch from `main`, keep changes scoped, and open a PR describing *what* changed and *why*.
Run the component checks before pushing (Go: `gofmt` / `go test ./...`; Python: `black` / `pytest`)
and make sure CI passes. Full details and the required checks are in
[`.github/CONTRIBUTING.md`](https://github.com/AMD-AGI/Primus-SaFE/blob/main/.github/CONTRIBUTING.md).

## Documentation contributions

The docs are Markdown under `docs-site/docs/`. To preview locally:

```bash
cd docs-site
npm install
npm run start
```

Use the **Edit this page** link at the bottom of any page to jump straight to its source. Keep
pages customer-focused; link to detail rather than duplicating it.

This site is also a **test suite**: pages carry reader-invisible `<!-- @test ... -->` annotations
that an agent executes against a live console, so the docs stay verified against the product. If
you change a documented procedure, update its annotation too. The convention and the agent contract
are in `docs-site/AGENTS.md`.

## License & security

Contributions are licensed under [Apache 2.0](https://github.com/AMD-AGI/Primus-SaFE/blob/main/LICENSE).
For security issues, do **not** open a public issue — follow the security policy in the repository.
