# Primus-SaFE Documentation

The documentation site for **Primus-SaFE** — AMD's full-stack platform for large-scale
model training and inference on AMD GPU clusters. Built with
[Docusaurus](https://docusaurus.io/) and published to GitHub Pages at
**https://amd-agi.github.io/Primus-SaFE/**.

## Written for humans *and* agents

These docs are written to be usable by two kinds of reader at once:

- **Humans** follow each page as a runbook: concrete, ordered steps with a clear
  "what a healthy result looks like" at every stage.
- **AI agents** can act on the same pages. Because every step is explicit and states
  its own success criteria, you can hand a page — or the whole site — to a coding agent
  and have it **do the work for you**: provision a cluster, install the platform, run a
  training job, set up observability, and verify each step against the documented result.

In other words: read it yourself, or point your agent at it and let it set things up on
your behalf. The same prose serves both.

## Structure

**Getting Started → Concepts → Tasks → Administration → Troubleshooting → FAQ →
Contributing.** Start at [Getting Started](https://amd-agi.github.io/Primus-SaFE/getting-started/prerequisites)
for install and your first job.

## Local development

Prerequisites: Node.js >= 18 (Node 22 also works).

```bash
cd docs-site
npm install
npm start          # dev server at http://localhost:3000 with hot reload
```

## Build

```bash
npm run build      # outputs the static site to ./build
npm run serve      # serve the built site locally
```

## Versioning

Docs live in `docs/` and are served as the current version. When the product cuts a
release, snapshot them:

```bash
npm run docusaurus docs:version 1.0
```

This freezes `docs/` into `versioned_docs/version-1.0/` and adds a version dropdown to
the navbar. Continue editing `docs/` for the next release.

## Deployment

The site deploys via GitHub Actions to GitHub Pages on merge to the default branch.
Hosting config (`url`, `baseUrl`, `organizationName`, `projectName`) lives in
`docusaurus.config.ts`.

## Contributing

See the [Contributing guide](https://amd-agi.github.io/Primus-SaFE/contributing). Edit
pages under `docs/`; the "Edit this page" link on each published page points back to its
source here.
