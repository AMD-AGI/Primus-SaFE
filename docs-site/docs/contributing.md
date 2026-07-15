---
sidebar_position: 10
title: Contributing
---

# Contributing

Contributions to Primus-SaFE — code and docs — are welcome. The canonical guide (issues, PR
workflow, CI checks, CODEOWNERS, security policy) lives in
[`.github/CONTRIBUTING.md`](https://github.com/AMD-AGI/Primus-SaFE/blob/main/.github/CONTRIBUTING.md);
this page is a quick orientation.

It is written to serve two audiences at once:

- **For you (the reader):** a short map of the repository and how to send a change.
- **For an AI agent:** this is a meta/contributor page, **not product behavior**, so it is
  **n/a** — there is nothing here for an agent to test against a live install.

There is no separate test file and no invisible annotation on this page: the prose you
read is all there is. The only thing kept elsewhere is bookkeeping (priority, and any
known product bug), in the run contract `docs-site/AGENTS.md`.

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

This site is also a **test suite**: each page is a **dual-audience walkthrough** — the same prose a
customer reads is what an agent executes against a live console, with the expected outcomes stated
in plain language (no hidden test layer). If you change a documented procedure or its stated
result, the test changes with it. The agent operating contract is in `docs-site/AGENTS.md`.

## License & security

Contributions are licensed under [Apache 2.0](https://github.com/AMD-AGI/Primus-SaFE/blob/main/LICENSE).
For security issues, do **not** open a public issue — follow the security policy in the repository.
