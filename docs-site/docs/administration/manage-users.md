---
sidebar_position: 1
title: Add users & assign access
---

# Add users & assign access

This page covers **who can log in** and **what they can use**. In Primus-SaFE these are two
separate steps:

1. **Identity** — how an account gets into the system (a local password account, an SSO
   account, or an automation key).
2. **Access** — which workspaces and platform-wide role that account is granted.

A brand-new account (however it was created) can do almost nothing until an admin grants it
workspace access — the only exception is a [default/public workspace](/concepts/workspace),
which every user can see.

This page is written to serve two audiences at once:

- **For you (the reader):** the console steps say *what* to click and *what a healthy result
  looks like*, so you can create an account and confirm exactly what it can and cannot do.
- **For an AI agent:** the golden path below is concrete and ordered, and each outcome —
  including the **permission boundary** (a regular user's view is *scoped* to what they've been
  granted, so their resource lists are empty until an admin grants them access) — is stated in
  plain language, so you can perform it and judge success without any hidden instructions.

There is no separate test file and no invisible annotation on this page: **the prose you read is
the test an agent runs.** The only thing kept elsewhere is bookkeeping (priority, personas, and
any known product bug), in the run contract `docs-site/AGENTS.md`.

## Before you start

This is an **admin** walkthrough that also signs in as a freshly created regular user. You need:

- A **running cluster** you can reach, with the console signed in as a **system-admin**.
- The ability to **sign out and back in** as a different account (the golden path creates a user
  and logs in as them).

> **Agent:** confirm you are signed in as a system-admin and the cluster is reachable before
> Step 1. If you cannot reach a running cluster or have no admin session, report **BLOCKED**
> (missing `running-cluster` / admin access) rather than failing a step. Do not modify or delete
> pre-existing users — only the uniquely-named user you create here.

## The identity model

Every account has a **type** and a set of **roles**.

| User type | How they log in | Has a password? |
|-----------|-----------------|-----------------|
| `default` | Username + password | Yes |
| `sso`     | Enterprise SSO (OIDC, e.g. Okta) | No — the identity provider authenticates them |

| Role | What it grants |
|------|----------------|
| `default` | Regular user. Can only act in workspaces they've been granted. |
| `system-admin` | Full control of all resources (users, workspaces, nodes, clusters). |
| `system-admin-readonly` | Can view everything platform-wide, but cannot create/update/delete. |

Roles and workspace grants are **independent of** how the user authenticates — an SSO user can
be a `system-admin`, and a local user can be a regular member.

## Golden path — create a user and verify their access

This is the executable walkthrough an agent performs end to end: create a regular local user,
sign in as them, confirm they land with a **limited, scoped view** (no access to resources they
haven't been granted), then freeze them and confirm they can no longer sign in.

*(A regular user may still see some **System** pages — the boundary is that their content is
**scoped** to their grants, not that the menu is hidden. The positive path — grant the user a
workspace, then have them act in it — isn't written up as steps here; an agent should verify the
scoped visibility in the table below and presence-check the rest, rather than drive a full grant
flow.)*

### Step 1 — Create a local user

1. In the left sidebar, expand **System** and click **Users**.
2. Click **Create** and fill in a **name**, **email**, and **password**. Use a unique name so
   repeated runs don't collide. (An agent names created resources per the run contract's cleanup
   convention.)
3. Click save.

Healthy (pass): the new user appears in the **Users** list with type `default`. If instead the
create dialog rejects the name as already taken, pick a fresh suffix — that is an input problem,
not a platform fault.

### Step 2 — Sign in as the new user

Sign out of the admin account and sign in at the password form (`/login-admin`, or `/login` when
SSO is disabled) with the name and password you just set.

- **Healthy (pass):** the new user can sign in and reaches the console.
- **If instead the sign-in is rejected** with correct credentials, that means account creation
  didn't actually take effect (fail).

### Step 3 — Confirm the account has a limited, scoped view

While signed in as the new default user, check what is visible. A regular user is **not** an
admin, but the console is **not** a locked-down blank app either — it is **scoped to what the user
has been granted**, and a brand-new user has no grants yet. The healthy result is *limited,
empty-for-now visibility*:

- Only **public / granted** workspaces are listed (a brand-new user has no grants, so only
  public/default workspaces appear).
- Where a **System** page is reachable to a regular user (for example **System → Nodes**), it
  shows **only the resources the user is entitled to**. For an unassigned user that list is
  **empty** — an empty Nodes list here is **expected**, not a bug.
- The user cannot see or act on **other tenants'** resources (nothing outside their grants).

If instead this default user can see or manage resources **outside their grants** — e.g. a
**populated** Nodes list, or another user's data — that is a privilege-boundary failure; report it.

> **Agent:** the healthy signal is **scoping**, not the *absence* of the System menu. Do **not**
> fail the page just because a **System** / **Nodes** item appears in the nav or the route loads —
> a regular user is allowed a limited System view. Check that the *content* is **empty/limited**
> for this unassigned user. (Known drift, not a failure: a list can render "No Data" even when the
> API has objects; for a brand-new user, empty is correct regardless.)

### Step 4 — Freeze the user (and confirm sign-in is blocked)

Sign back in as the admin, open the new user's row action, and **freeze** it (revoke access
without deleting). Then sign out and try to sign in as that user again.

- **Healthy (pass):** the frozen user can **no longer sign in**.
- **If instead they can still sign in** after being frozen, the freeze didn't take effect (fail).

### What you should see

> **Agent:** fill the table below from what you observe, show it to the user, and report
> **PASS** only if every row is healthy (including the negatives). Then **run cleanup**: as
> admin, delete the user via its row action, so the run is repeatable.

| Check | Healthy result | Found |
|---|---|---|
| New user appears in Users list | yes (type `default`) | _fill in_ |
| New user can sign in | yes | _fill in_ |
| Workspaces listed (as that user) | only public / granted | _fill in_ |
| System pages reachable to the user (e.g. Nodes) | allowed, but **scoped** — empty for an unassigned user | _fill in_ |
| Other tenants' resources visible (e.g. a populated Nodes list) | no — nothing outside their grants | _fill in_ |
| After freeze, user can sign in | no | _fill in_ |
| Cleanup (user deleted) | done | _fill in_ |

## From the console (UI)

If you prefer the web console over the API, an admin manages people under **System → Users**.
The same three things apply: create the account, then grant **roles** and **workspaces**.

1. **Open the Users page.** In the left sidebar, expand **System** and click **Users**.
2. **Create a local user.** Click **Create** and fill in name, email, and a password. (SSO users
   appear here automatically after their first login — you don't create them.)
3. **Assign access.** Open a user's row action (**Edit**) and set their **roles** and the
   **workspaces** (member) / **managed workspaces** (manager) they should have, then save.
4. **Freeze or delete.** The same row actions let you freeze (revoke access without deleting) or
   delete a user.

The sections below explain each step in detail, with the equivalent API calls for automation. The
console is the primary path; the REST calls are the scripted equivalent.

## Approach 1 — Local user (username + password)

Best for: small/offline deployments, break-glass admin access, or environments without an
enterprise IdP.

A local user is type `default` and signs in at the console's password form (`/login-admin`,
or `/login` when SSO is disabled). There are two ways to create one:

**a) Self-registration.** If the console exposes the registration page (`/register`), a person
can create their own account. Registration is a public endpoint:

```bash
curl -X POST https://<your-console>/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{ "name": "zhangsan", "email": "zhangsan@example.com", "password": "<strong-password>" }'
```

The `name` must be unique. After this, the user **can log in but has no workspace access** yet
(see [Assign access](#assign-workspace-access-and-roles)).

**b) Admin-created.** An admin creates the account the same way, then immediately assigns roles
and workspaces with a follow-up update (next section).

:::note
Self-registration creates a *regular* (`default`) user only. Promoting someone to
`system-admin` always requires an existing admin.
:::

## Approach 2 — SSO user (enterprise OIDC / Okta)

Best for: production deployments tied to a corporate identity provider (e.g. Okta).

SSO is enabled in the **server configuration**, not per-user. When configured, the platform
exposes it via `GET /api/v1/envs`:

```json
{ "ssoEnable": true, "ssoAuthUrl": "https://accounts.example.com/oauth2/authorize?..." }
```

When `ssoEnable` is true, the console's `/login` page redirects straight to the IdP. The flow
is standard OIDC authorization-code (scopes `openid profile email`):

1. User goes through IdP's authentication process.
2. The IdP redirects back with a `code`; the console exchanges it (`POST /api/v1/login` with
   `type: sso`).
3. **First time only:** the platform auto-provisions a `sso` user from the ID-token claims
   (email/name). No password is stored. On later logins, the user's email/name are re-synced
   from the token.

So you do **not** pre-create SSO users — they appear automatically the first time they log in.
What you *do* still have to do is grant them access:

:::warning Newly auto-provisioned SSO users have no access
A first-time SSO user lands with **no roles beyond `default` and no workspaces**. Until an admin
grants access, they can only see public workspaces. Plan to grant access right after a new
teammate's first login (or pre-arrange it).
:::

To turn SSO on, the operator sets the OIDC endpoint, client ID/secret, and redirect URI in the
server config (`ssoEnabled`, `ssoEndpoint`, `ssoClientId`, `ssoClientSecret`, `ssoRedirectURI`).

## Approach 3 — API keys (for automation, not people)

Best for: scripts, CI/CD, and agents that act without an interactive login. An API key is not
a separate user — it is a bearer credential that **inherits the permissions of the user who
created it**. Keys start with `ak-` and are sent as `Authorization: Bearer ak-...`.

See [Manage access & quota → API keys](/administration/manage-access-and-quota#api-keys-for-scripts-ci-agents)
for how to mint, scope (TTL, IP allowlist), and revoke them.

## The bootstrap administrator

The installer creates an initial `system-admin` account so you can sign in the very first time
and create/authorize everyone else. Its default credentials are username **`root`** / password
**`root`** — **change the password immediately** after your first sign-in. Treat this account as
break-glass: keep its credentials safe and prefer day-to-day administration through named admin
accounts.

## Assign workspace access and roles

This is the step that actually lets a user do work. Two levels of workspace access exist:

- **Member** (`workspaces`) — submit and manage *their own* workloads, view others' in the same
  workspace, and create low/medium-priority jobs.
- **Manager / workspace-admin** (`managedWorkspaces`) — manage *everyone's* workloads, change
  workspace config, grant access to others, and use the highest priority. **Granting someone
  manager also grants them access.**

Update a user with the workspaces and/or roles they should have:

```bash
curl -X PATCH https://<your-console>/api/v1/users/<userId> \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "roles": ["default"],
    "workspaces": ["prod-cluster-ai-team", "prod-cluster-dev-team"]
  }'
```

**Who can grant what:**

- Changing **roles** or a user's **workspace list** requires `system-admin`.
- A **workspace manager** can grant other users access to *their own* workspace.
- A **default/public** workspace (`isDefault`) is visible to everyone with no grant needed.

The underlying access model is described in
[Workspace → Access model](/concepts/workspace#access-model-brief).

## Freeze or remove a user

- **Freeze** (revoke access without deleting) — set `restrictedType: 1` via the same PATCH; a
  frozen user cannot use the system. Set back to `0` to restore.
- **Delete** — `DELETE /api/v1/users/<userId>` (system-admin only).

```bash
# Freeze
curl -X PATCH https://<your-console>/api/v1/users/<userId> \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{ "restrictedType": 1 }'
```

## Quick reference

| Task | Endpoint | Who |
|------|----------|-----|
| Create local user | `POST /api/v1/users` | Anyone (public) |
| Log in (password) | `POST /api/v1/login` (`type: default`) | — |
| Log in (SSO) | `POST /api/v1/login` (`type: sso`, `code`) | — |
| Grant roles / workspaces | `PATCH /api/v1/users/{id}` | `system-admin` |
| Grant access to one workspace | `PATCH /api/v1/users/{id}` | workspace manager |
| Freeze user | `PATCH /api/v1/users/{id}` (`restrictedType: 1`) | `system-admin` |
| Delete user | `DELETE /api/v1/users/{id}` | `system-admin` |
| List users (optionally by workspace) | `GET /api/v1/users?workspaceId=...` | authenticated |
