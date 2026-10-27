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
| `builtin` | Hands the request to the built-in mode. Put it last to keep a local administrator able to sign in, including while the introspection endpoint is down. |

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
 "scope": "safe.read", "jti": "j-9", "exp": 1893456000, "act": {"client_id": "svc-bot"}}
```

`active: false` rejects the credential, whatever else the answer says. An answer without `active`, any other HTTP status (including `401` and `403` for the apiserver's own credentials), or a body that is not a JSON object means the introspection did not happen.

* `sub` is the SaFE user id; the endpoint decides it. An active answer without one is refused.
* `name` (else `username`), `email` (else `username` when it looks like an email), `jti`, and `exp` (unix seconds) describe the credential.
* `groups` and `roles` are both group names for `auth.group_mappings`, once each; a name no mapping lists grants nothing.
* `act` marks a delegated credential: the actor is `act.client_id`, else `act.sub`. An `act` that names no actor is refused rather than read as a direct login. `scope` is the space-separated scope of a delegated credential.
* `restricted: true` freezes the user for the request.
* `iss`, when present and when `issuer` is configured, must match it, or the credential is rejected.

### stale_if_error

If the introspection endpoint cannot answer, a credential it **accepted** before keeps working for `stale_if_error` after its `cache_ttl` (default `30m`, set `-1` to turn it off), but never past its own `exp`. A credential it never accepted gets `503`. Revocation during an outage therefore takes effect after at most `cache_ttl + stale_if_error`. Each such request increments `safe_apiserver_auth_stale_served_total`, which is worth alerting on.

## Users and roles

* **Just-in-time users.** The first time an authenticated `uid` is seen, SaFE creates the user object (name is the `uid`, which must be a valid resource name; email and display name become annotations). Later requests only refresh those annotations. No role is written.
* **Groups map to roles at request time.** `auth.group_mappings` maps identity groups to SaFE roles; the result is computed for each request and never stored, so removing a user from a group takes effect on the next request. The effective roles are the user object's `roles` united with the mapped ones. The `workspace-admin` role in a mapping means managing the listed `workspaces`; any mapping with `workspaces` also grants access to them.

```yaml
auth:
  mode: external
  authenticators:
    - {type: oidc, issuer: "https://idp.example.com", audience: safe-apiserver}
    - {type: introspection, issuer: "https://idp.example.com", client_id: safe-apiserver, client_secret_file: /etc/secrets/auth/client-secret}
    - {type: builtin}
  group_mappings:
    - {group: platform-admins, roles: [system-admin]}
    - {group: team-a-leads, roles: [workspace-admin], workspaces: [team-a]}
  login_url: "https://console.example.com/sign-in"
```

Mapped roles and workspaces are honoured by the access controller (`Authorize`, `AuthorizeSystemAdmin`, role lookup) and by `GET /users/self`. A handful of handlers that read the stored user object directly (for example some model and workspace-label checks) still see only the stored roles; they fail closed.

## Delegated tokens

A token that carries an `act` claim (RFC 8693) means a party is acting on behalf of the user; SaFE records the actor, and restricts it, whoever issued it:

* **Audit.** `audit_logs` has `actor_client_id` and `token_jti`. Reads made with a delegated token are recorded too (without bodies), not only writes.
* **Capability exclusions.** A delegated token can never call API-key, Secret, user-management, SSH-key-writing, gateway-credential or token-verification routes, whatever its scope. The list is pinned by tests against the real route table; a new route whose path mentions a credential fails the test until it is classified.
* **Scope.** A delegated token needs `safe.api` (everything the user may do, minus the exclusions) or `safe.read` (reads only) in its `scope` claim; without one it is refused.

A malformed `act` claim is rejected rather than read as a direct login.

## Database

`audit_logs` gains two nullable columns (`actor_client_id`, `token_jti`). The chart adds them with `ADD COLUMN IF NOT EXISTS`; apply the database job before rolling the apiserver.

## Rolling back

Set `auth.mode: builtin`. No user object or table is changed by either mode.
