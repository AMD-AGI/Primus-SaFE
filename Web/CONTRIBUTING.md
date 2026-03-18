## Contributing (Primus Frontend)

This repository is a **pnpm workspace** that currently hosts two independent frontend apps:

- `apps/safe`: Safe UI
- `apps/lens`: Lens UI

They live in the same repo for open-sourcing and collaboration convenience. **UI/tech-stack unification is not required.**

### Prerequisites

- Node.js: recommended 22.x
- pnpm: pinned via `packageManager` in the root `package.json`

### Install

From the repo root:

```bash
pnpm install
```

### Develop & Build

Safe:

```bash
pnpm dev:safe
pnpm build:safe
```

Lens:

```bash
pnpm dev:lens
pnpm build:lens
```

You can also use workspace filters (recommended):

```bash
pnpm --filter ./apps/safe dev
pnpm --filter ./apps/lens dev
```

### Test

Tests use [Vitest](https://vitest.dev/).

```bash
pnpm test          # Run all tests
pnpm test:safe     # Run Safe tests only
pnpm test:lens     # Run Lens tests only
```

When adding new utility functions or services, please include unit tests in a `__tests__/` directory alongside the source file.

### Add dependencies to an app (important)

Add dependencies to the correct app, not the workspace root:

```bash
pnpm --filter ./apps/safe add <pkg>
pnpm --filter ./apps/safe add -D <pkg>

pnpm --filter ./apps/lens add <pkg>
pnpm --filter ./apps/lens add -D <pkg>
```

### Common pitfalls

- **Do not commit `package-lock.json`**: this repo uses `pnpm-lock.yaml`.
- **PowerShell + `--filter`**: if you use `@scope/name`, quoting can be tricky. Prefer path filters (e.g. `--filter ./apps/safe`).

