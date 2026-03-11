# Primus Frontend

Frontend monorepo for the Primus platform.

## Structure

```text
apps/
  safe/        # Control plane & workload management UI
  lens/        # Lens UI
packages/      # Shared frontend packages (future)
```

## Prerequisites

- Node.js: recommended Node 22.x
- pnpm: this repo pins pnpm via `packageManager` in `package.json`

## Install

```bash
pnpm install
```

## Develop

### Safe

```bash
pnpm dev:safe
```

### Lens

```bash
pnpm dev:lens
```

## Test

```bash
pnpm test          # Run all tests
pnpm test:safe     # Run Safe tests only
pnpm test:lens     # Run Lens tests only
```

## Build

```bash
pnpm build:safe
pnpm build:lens
```

## Workspace notes

- This is a pnpm workspace (`pnpm-workspace.yaml`). Add dependencies to the correct app:

```bash
pnpm --filter ./apps/safe add <pkg>
pnpm --filter ./apps/lens add <pkg>
```
