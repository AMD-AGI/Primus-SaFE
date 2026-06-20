# Primus-SaFE Documentation Site

GA-ready documentation portal for Primus-SaFE, built with [Docusaurus](https://docusaurus.io/).

The information architecture follows the Kubernetes-ecosystem standard
(modeled on [karpenter.sh/docs](https://karpenter.sh/docs/)):
**Getting Started → Concepts → Tasks → Agentic → Reference → Operations →
Troubleshooting → FAQ → Contributing**.

## Prerequisites

- Node.js >= 18 (the repo's `Web/` app pins Node 22; that works too)

## Local development

```bash
cd docs-site
npm install
npm start
```

This starts a local dev server at http://localhost:3000 with hot reload.

## Build

```bash
npm run build      # outputs static site to ./build
npm run serve      # serve the built site locally
```

## Versioning

Docs live in `docs/` and are served as the "current" (unreleased) version.
When the product cuts a release, snapshot the docs:

```bash
npm run docusaurus docs:version 1.0
```

This freezes the current `docs/` into `versioned_docs/version-1.0/`, and a version
dropdown appears in the navbar automatically. Continue editing `docs/` for the next
release.

## Deployment

Intended to deploy via GitHub Actions to GitHub Pages (or Netlify) on merge.
Update `url` / `baseUrl` in `docusaurus.config.ts` to the real published URL before GA.
