# External Authentication

## Overview

By default (`auth.mode: builtin`) the apiserver authenticates users itself: local accounts, built-in sessions, generic OIDC sign-in and API keys. Nothing in this document applies, and nothing changes.

With `auth.mode: external` the apiserver instead asks a configurable chain of authenticators who a request belongs to, the way Kubernetes does with `--oidc-*` flags and token authentication webhooks, using the standards any identity provider implements: OIDC discovery, JWKS and OAuth 2.0 token introspection (RFC 7662). **Authentication** moves out; **authorization** (roles, workspaces, resource ownership) stays in SaFE and is evaluated by the same access controller as before.

## The authenticator chain

`auth.authenticators` is tried in order. The first authenticator that accepts the credential decides the identity; if none does, the answer is `401`. If one could not decide (for example the introspection endpoint is unreachable) and no other accepted, the answer is `503`: an unavailable authenticator is never a pass, and never cached as a rejection.

| Type | What it does |
|---|---|
| `oidc` | Validates a JWT from one issuer: signature (keys from `jwks_url`, or the issuer's discovery document), `iss`, `aud` (required), `exp`, `nbf`, with `clock_skew` leeway. Keys are cached (`jwks_cache_ttl`), refreshed when an unknown key id appears, and the cached keys keep working if the issuer's key endpoint is down. A JWT from another issuer is not judged by this entry, so several issuers can share a chain. |
| `introspection` | Asks an OAuth 2.0 token introspection endpoint (RFC 7662) at `url` (or the `introspection_endpoint` of `issuer`'s discovery document) who a credential belongs to, for credentials SaFE cannot interpret itself (opaque tokens such as API keys, session cookies). The apiserver authenticates to the endpoint as `client_id` with the secret in the file `client_secret_file` (a mounted Secret, read on every call so rotation needs no restart). Answers are cached: accepted credentials for `cache_ttl` (default 30s), rejected ones for `cache_negative_ttl` (default 5s); concurrent requests with one credential share one call. |
| `builtin` | Hands the request to the built-in mode, which keeps accepting the sessions and API keys it issued before the switch. The identity it authenticates holds no platform role (see [Users and roles](#users-and-roles)). It does not bring back sign-in: see "What the apiserver stops offering". |

### Introspection protocol

Request (`POST`, `application/x-www-form-urlencoded`, HTTP Basic with the client id and secret):

```
token=<bearer token, or the session cookie's value when there is no bearer token>
user_type=<userType, when the request has one>
client_ip=203.0.113.7
```

`user_type` and `client_ip` are extensions: the first says what kind of session an opaque token is, the second is the caller's address, for an API key's IP allow list. Response (`200`, JSON):

```json
{"active": true, "iss": "https://iam.example.com", "sub": "user-id", "username": "alice@example.com", "name": "Alice",
 "email": "alice@example.com", "groups": ["platform-admins"], "roles": ["platform-admins", "default"],
 "aud": "safe-apiserver", "scope": "safe.read", "jti": "j-9", "exp": 1893456000, "act": {"client_id": "svc-bot"}}
```

`active: false` rejects the credential, whatever else the answer says. An answer without `active`, any other HTTP status (including `401` and `403` for the apiserver's own credentials), or a body that is not a JSON object means the introspection did not happen.

* `sub` is the SaFE user id; the endpoint decides it. An active answer without one is refused.
* `name` (else `username`), `email` (else `username` when it looks like an email), `jti`, and `exp` (unix seconds) describe the credential.
* `groups` and `roles` are both group names for `auth.group_mappings`, once each; a name no mapping lists grants nothing.
* `act` marks a delegated credential: the actor is `act.client_id`, else `act.sub`. An `act` that names no actor is refused rather than read as a direct login. `scope` is the space-separated scope of a delegated credential.
* `restricted: true` freezes the user for the request.
* `iss`, when present and when `issuer` is configured, must match it, or the credential is rejected.
* `aud` (a string or an array) must contain the configured `audience`, which is required, or the credential is rejected. An introspection endpoint answers `active: true` for any valid token it issued, whichever client the token was minted for, so without this check a token issued to another relying party of the same issuer could be replayed against SaFE. When an answer has no `aud`, its `client_id` stands in for it and must equal `audience`. An answer that names no audience at all (no `aud` and no `client_id`, as the answer for an API key usually is) is rejected unless `allow_missing_audience: true`; an answer that names another audience is rejected either way. An `aud` that is neither a string nor an array of strings is treated like an unreadable answer (`503`).

### stale_if_error

If the introspection endpoint cannot answer, a credential it **accepted** before keeps working for `stale_if_error` after its `cache_ttl` (default `30m`, set `-1` to turn it off), but never past its own `exp`. A credential it never accepted gets `503`. Revocation during an outage therefore takes effect after at most `cache_ttl + stale_if_error`. Each such request increments `safe_apiserver_auth_stale_served_total`, which is worth alerting on.

## Users and roles

* **Just-in-time users.** The first time an authenticated `uid` is seen, SaFE creates the user object (name is the `uid`, which must be a valid resource name; email and display name become annotations). Later requests only refresh those annotations. No role is written.
* **Groups map to roles at request time.** `auth.group_mappings` maps identity groups to SaFE roles; the result is computed for each request and never stored, so removing a user from a group takes effect on the next request. The `workspace-admin` role in a mapping means managing the listed `workspaces`; any mapping with `workspaces` also grants access to them.
* **The platform roles come only from the identity.** `system-admin` and `system-admin-readonly` are held by a request if and only if a group mapping grants them. The roles stored on the user object are ignored for these two, so an administrator whose group is removed at the identity provider stops being one when the credential is next authenticated afresh, even though the object still lists the role, and restoring the group restores the role. For an `introspection` credential that is within `cache_ttl` (30s by default). An `oidc` JWT carries its groups claim fixed until it expires, so a revoked administrator keeps the role until the token's `exp`; keep access tokens short-lived, or use `introspection` where revocation must take effect promptly. Nothing is written to the user object, so `auth.mode: builtin` finds it as it was. Every other role, the workspaces and the restriction are the user object's united with the mapped ones. A user who holds no platform role is an ordinary user and holds the `default` role. This applies to every request in external mode, including one the chain hands to the `builtin` entry: no identity provider vouched for it, so the platform roles stored on its user object do not count. There is no local-account exception; to sign in with local accounts, use `auth.mode: builtin`.
* **The administrator path is the identity provider.** In external mode an administrator is a user whose identity-provider credential carries a group that a mapping grants `system-admin`, and signs in through the identity provider. A built-in credential (a session or API key that a local account already holds) is an ordinary user, so a local administrator gets `403` on the administrator pages and `POST /auth/verify` returns no platform role for it.
* **`POST /auth/verify` runs the same chain.** The services behind the apiserver that authorize through it send the credential in the request body (a session cookie, an API key or a bearer token), and the endpoint authenticates it with `auth.authenticators`, in order, exactly as the apiserver does for a request: an identity provider's credential is answered with the user's roles as the apiserver computes them (the mapped `system-admin` included), a credential the chain rejects gets `401`, an authenticator that could not decide gets `503`, and a credential the chain hands to the `builtin` entry is answered as above, with no stored platform role. A delegated token (one with an `act` claim) is refused with `403`, as for the other credential routes. The endpoint is legacy: services should move to `GET /users/self` with the user's own credential, which returns the same roles and needs no internal token.

```yaml
auth:
  mode: external
  authenticators:
    - {type: oidc, issuer: "https://idp.example.com", audience: safe-apiserver}
    - {type: introspection, issuer: "https://idp.example.com", audience: safe-apiserver, client_id: safe-apiserver, client_secret_file: /etc/secrets/auth/client-secret}
  group_mappings:
    - {group: platform-admins, roles: [system-admin]}
    - {group: team-a-leads, roles: [workspace-admin], workspaces: [team-a]}
  login_url: "https://console.example.com/sign-in"
```

Mapped roles and workspaces are part of the user every handler resolves for the request (`AccessController.GetRequestUser`), so the access controller (`Authorize`, `AuthorizeSystemAdmin`, role lookup), the checks handlers make on the user directly (model visibility, workspace access, the sandbox environment Secret, reserved workspace labels), `GET /users/self` and `GET /users/self/workspaces` all agree. That user is a copy for the request; it is never written back to the user object. For the same reason a `PATCH /users/{id}` that edits the caller's own workspaces stores only the workspaces the caller's groups do not grant (the console's row lists the granted ones too, and sends them back with an edit): a granted workspace is left out of the stored list unless the record already holds it, so it ends with the group. The web shell is an HTTP request and keeps the roles and workspaces of its identity like any other. An SSH session is not an HTTP request: it is authenticated by public key, not by an identity provider, so it follows the same rule as a built-in credential: the platform roles stored on the user object do not count, and the user holds the `default` role and the workspaces and other roles of the record.

## Upgrading from a release that kept local administrators

**Breaking change for deployments that relied on a local administrator.** Earlier releases told operators to put `builtin` last in `auth.authenticators` to keep a local administrator able to sign in, including while the introspection endpoint is down. That exception is gone: in external mode the platform roles (`system-admin`, `system-admin-readonly`) held by a request come only from `auth.group_mappings`, and the roles stored on user objects, such as that of a local break-glass administrator, are not counted. After the upgrade such an account still authenticates through the `builtin` entry (its sessions and API keys keep working) but is an ordinary user, and `POST /auth/verify` and SSH sessions agree. A local administrator therefore cannot administer the platform through the console either: every administrator signs in through the identity provider. The services that authorize through `POST /auth/verify` now see the identity provider's administrators there (the endpoint runs the same chain as the apiserver) and no local one.

Before upgrading a deployment in external mode:

* Make sure every administrator is granted `system-admin` (or `system-admin-readonly`) by a group mapping at the identity provider. There is no local fallback while the identity provider is down.
* If a local administrator is needed as a break-glass path, that is `auth.mode: builtin`: set it back (a configuration change and an apiserver roll) to sign in with local accounts. No user object or table is changed by either mode.

The apiserver logs a warning at start-up in external mode, and names the `builtin` entry when the chain has one, so the change is visible before an outage and not during it.

## What the apiserver stops offering

With `auth.mode: external` the identity provider owns sign-in, accounts, API keys and platform roles, so the apiserver refuses the ways of creating them itself, with `403`, for every caller and every account type (there is no local-account exception):

| Request | Why |
|---|---|
| `POST /api/v1/login` (password and SSO code) | sign-in belongs to the provider |
| `POST /api/v1/users` (registration) | accounts are created by the provider's first sign-in |
| `POST /api/v1/apikeys` | keys are issued by the provider |
| `PATCH /api/v1/users/{id}` that carries a `password` | there are no passwords in this mode, and one set here would be unknown to its owner after a rollback to `builtin` |
| `PATCH /api/v1/users/{id}` that grants `system-admin` or `system-admin-readonly` the stored user does not have | platform roles come from the provider through `group_mappings`; a patch that leaves them as they are (other fields, other roles) is accepted, and so is one that only removes a stored platform role, which cleans up a record that external mode ignores anyway |

`GET /api/v1/envs` reports `authMode`, and the console hides the password form, registration, password change, API key creation and role editing accordingly. In this mode `/envs` does not advertise the built-in sign-in either: `ssoEnable` is `false` and `ssoAuthUrl` is empty unless `auth.login_url` is set, in which case they are that URL. Listing keys (`GET /apikeys`), deleting them (`DELETE /apikeys/{id}`) and everything else is unchanged: a key issued before the switch is still accepted through the `builtin` entry, so it must stay possible to revoke it.

Because a local sign-in is not available, backing out of an outage of the provider is a configuration change, not a login: set `auth.mode: builtin` (see below).

## Delegated tokens

A token that carries an `act` claim (RFC 8693) means a party is acting on behalf of the user; SaFE records the actor, and restricts it, whoever issued it:

* **Audit.** `audit_logs` has `actor_client_id` and `token_jti`. Reads made with a delegated token are recorded too (without bodies), not only writes.
* **Capability exclusions.** A delegated token can never call API-key, Secret, user-management, SSH-key-writing, gateway-credential or token-verification routes, whatever its scope. The list is pinned by tests against the real route table; a new route whose path mentions a credential fails the test until it is classified.
* **Scope.** A delegated token needs `safe.api` (everything the user may do, minus the exclusions) or `safe.read` (reads only) in its `scope` claim; without one it is refused. A read is judged by what the request can do, not by its HTTP method: the web shell is a `GET` that upgrades into an interactive exec session, so a request that asks the server to switch protocols (an `Upgrade` header or a `Connection: upgrade` token, whichever route it goes to), and the routes that open a shell, exec, attach or port-forward session, need `safe.api`. A test reads the handlers that upgrade the connection and checks that their routes are classified.

A malformed `act` claim is rejected rather than read as a direct login.

## Database

`audit_logs` gains two nullable columns (`actor_client_id`, `token_jti`). The chart adds them with `ADD COLUMN IF NOT EXISTS`; apply the database job before rolling the apiserver.

## Keeping the settings across upgrades

`bootstrap/upgrade.sh`, which the CD job runs too, renders the chart from the chart defaults plus what `.env` says, so an `auth.*` setting that is not in `.env` goes back to its default on the next upgrade. `.env` therefore carries them (all optional; a JSON value is single-quoted):

```
auth_mode=external
auth_login_url=https://console.example.com/sign-in
auth_client_secret=primus-safe-auth
auth_authenticators='[{"type":"oidc","issuer":"https://idp.example.com","audience":"safe-apiserver"}]'
auth_group_mappings='[{"group":"platform-admins","roles":["system-admin"]}]'
auth_key_actors='["client-a"]'
```

They set `auth.mode`, `auth.login_url`, `auth.client_secret` (the name of the Secret that holds the introspection client secret, not the secret itself), `auth.authenticators`, `auth.group_mappings` and `auth.key_actors`. Values that are not usable (a JSON key that is not an array, a mode that is neither `builtin` nor `external`, `external` without authenticators) stop the upgrade before it changes anything; the script needs `jq` when `.env` sets any of them (`auth_mode=builtin` on its own excepted) and to upgrade a release that is already external; a release that is not external upgrades without it.

An upgrade never silently turns external authentication off. When the running release has `auth.mode: external` and `.env` does not set a key the release has (the mode itself, or any of the others with a value), the script exits non-zero before it applies anything and names the missing keys; copy them from `helm -n primus-safe get values primus-safe -o json | jq .auth`. It also refuses when it cannot read the running release, because an unknown mode is not a safe one.

## Rolling back

Set `auth.mode: builtin` and roll the apiserver. No user object or table is changed by either mode, and the endpoints listed under "What the apiserver stops offering" work again.

With `bootstrap/upgrade.sh` or the CD job, that is `auth_mode=builtin` in `.env`: the deliberate way back, which the check above always lets through. Leave the other `auth_*` keys in place, and the way forward is removing that line (or setting `auth_mode=external` again).
