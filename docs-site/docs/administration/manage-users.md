---
sidebar_position: 1
title: Add users & assign access
---

# Add users & assign access

> **Status:** Draft · **Owner:** _unassigned_ · **Source:** `SaFE/docs/apis/user.md`,
> `envs.md`, `api-key.md`; `apiserver/.../authority/sso_token.go`

This page covers **who can log in** and **what they can use**. In Primus-SaFE these are two
separate steps:

1. **Identity** — how an account gets into the system (a local password account, an SSO
   account, or an automation key).
2. **Access** — which workspaces and platform-wide role that account is granted.

A brand-new account (however it was created) can do almost nothing until an admin grants it
workspace access — the only exception is a [default/public workspace](/concepts/workspace),
which every user can see.

<!-- @test
scope: page
mode: contract
priority: P0
personas: [admin, member]
preconditions: [running-cluster]
do: follow this page's "From the console (UI)" steps to create a default user (unique name), then sign out and sign in as them
expect:
  - the new user can sign in
  - as that default user: no System admin section in the nav; Nodes/Clusters/Users not reachable; only public/granted workspaces listed
  - after an admin freezes them (row action), they can no longer sign in
cleanup: as admin, delete the user via its row action
-->
<!-- @test todo:
  - "Doc does not state the exact non-admin behavior on System pages (hidden nav vs redirect); make the prose explicit so the expect can be precise."
  - "Add the positive path: grant the user a workspace, then confirm they can act in it."
-->

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

## From the console (UI)

If you prefer the web console over the API, an admin manages people under **System → Users**.
The same three things apply: create the account, then grant **roles** and **workspaces**.

<!-- screenshot: System → Users list (sanitized — no real usernames/emails) — add image -->

1. **Open the Users page.** In the left sidebar, expand **System** and click **Users**.
2. **Create a local user.** Click **Create** and fill in name, email, and a password. (SSO users
   appear here automatically after their first login — you don't create them.)

<!-- screenshot: System → Users → Create dialog (empty form) — add image -->

3. **Assign access.** Open a user's row action (**Edit**) and set their **roles** and the
   **workspaces** (member) / **managed workspaces** (manager) they should have, then save.

<!-- screenshot: System → Users → Edit dialog showing Roles + Workspaces fields — add image -->

4. **Freeze or delete.** The same row actions let you freeze (revoke access without deleting) or
   delete a user.

The sections below explain each step in detail, with the equivalent API calls.

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

> **Not yet covered (capture so we don't lose it):**
> - [ ] Exact config keys / Helm values that enable SSO, with a worked Okta example.
> - [ ] Whether IdP groups can map to platform roles/workspaces automatically (today the
>       mapping appears to be manual — confirm).

## Approach 3 — API keys (for automation, not people)

Best for: scripts, CI/CD, and agents that act without an interactive login. An API key is not
a separate user — it is a bearer credential that **inherits the permissions of the user who
created it**. Keys start with `ak-` and are sent as `Authorization: Bearer ak-...`.

See [Manage access & quota → API keys](/administration/manage-access-and-quota#api-keys-for-scripts-ci-agents)
for how to mint, scope (TTL, IP allowlist), and revoke them.

## The bootstrap administrator

The installer creates an initial `system-admin` account (the "root" admin) so you can sign in
the very first time and create/authorize everyone else. Treat it as break-glass: keep its
credentials safe and prefer day-to-day administration through named admin accounts.

> **Not yet covered:**
> - [ ] Document the bootstrap admin's default name and where its initial password is set
>       (install `.env` / Helm value), and how to rotate it.

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

The underlying access model is described in [Workspace → Access model](/concepts/workspace#access-model-brief).

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

> **Not yet covered (capture so we don't lose it):**
> - [ ] Capture the sanitized screenshots for the [From the console](#from-the-console-ui) steps
>       (Users list, Create dialog, Edit/roles+workspaces dialog).
> - [ ] Whether a workspace manager grants access from the Users page or the Workspace detail
>       page (confirm the actual UI path).
> - [ ] Audit trail for access changes (link to audit logs).
