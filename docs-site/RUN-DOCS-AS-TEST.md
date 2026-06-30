# How to run the docs-as-test suite

The docs-site **is** the test spec. An agent reads the invisible `<!-- @test ... -->` blocks in
`docs/**/*.md` and executes them against a live Primus-SaFE. The full contract is in
[`AGENTS.md`](./AGENTS.md); this file is just the copy-paste trigger.

## Prerequisites

- A running Primus-SaFE test env you provide: seeded `root`/`root`, a few nodes, 1 cluster, and a
  workspace **with quota**. (Install/bring-up is verify-only — never tested.)
- The **`user-Playwright` MCP** enabled in the thread (Settings → Tools & MCP), with a browser
  installed: `npx playwright install --with-deps chrome` (or use `--browser=chromium`, which needs
  no sudo). Config lives in `~/.cursor/mcp.json`.
- Network access from the agent to the console URL.

## Trigger — paste into a brand-new thread

> Runtime values (console URL, admin login) live in a **local, gitignored** file — never in this
> doc. Copy `.docs-test.env.example` to `.docs-test.env` and fill it in. Don't paste real hosts or
> credentials into the prompt or any committed file.

```
Execute the docs-as-test suite. The docs-site IS the test spec.

1. Read docs-site/AGENTS.md (the operating contract).
2. Collect every <!-- @test ... --> block under docs-site/docs/**/*.md.
3. Load runtime params from docs-site/.docs-test.env (PRIMUS_CONSOLE_URL, PRIMUS_ADMIN_LOGIN);
   substitute them for {{baseUrl}} and the admin login. Do not print the credentials.
4. DRIVER: use ONLY the "user-Playwright" MCP server (browser_navigate/click/type/
   snapshot/take_screenshot). Do NOT use cursor-ide-browser.
5. UI-ONLY: perform every step and every assertion through the console UI — navigate, read
   the rendered page, click the documented controls. Do NOT call the REST API and do NOT use
   browser_evaluate fetch for assertions. (kubectl is allowed ONLY as an optional read-only
   ground-truth note when the UI looks wrong — never as the pass/fail check.)
   Start by browser_navigate to {{baseUrl}} and signing in with PRIMUS_ADMIN_LOGIN.
6. Run scope = RELEASE: all verify + behavior + contract blocks. Report PASS/FAIL/BLOCKED
   per expect, quote "doc says X / product does Y", screenshot on FAIL, run each cleanup.
7. Output a summary table (page · level · result) + a findings list.
```

## Variants

- **Docs-PR gate (fast):** change the run scope to *"only `verify` blocks plus the `behavior`
  blocks on pages changed in this PR."*
- **Allow API ground-truth (faster, less pure):** relax step 5 to *"assert through the UI, but
  you may use the Playwright browser_evaluate in-page fetch (e.g. fetch('/api/v1/...')) as a
  cheaper cross-check when a UI assertion is awkward."* Use only if you don't need pure black-box.
- **Different environment:** swap the two target values in step 3.

## Why name the server explicitly

Two browser MCPs are usually available (`user-Playwright` and the built-in `cursor-ide-browser`).
Naming `user-Playwright` and routing API assertions through its `browser_evaluate` guarantees the
whole run goes through the Playwright MCP under one authenticated session.
