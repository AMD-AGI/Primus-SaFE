# External Authentication

## Overview

By default (`auth.mode: builtin`) the apiserver authenticates users itself: local accounts, built-in sessions, generic OIDC sign-in and API keys. Nothing in this document applies, and nothing changes.

With `auth.mode: external` the apiserver instead asks a configurable chain of authenticators who a request belongs to, the way Kubernetes does with `--oidc-*` flags and webhook token authentication. **Authentication** moves out; **authorization** (roles, workspaces, resource ownership) stays in SaFE and is evaluated by the same access controller as before.

## The authenticator chain

`auth.authenticators` is tried in order. The first authenticator that accepts the credential decides the identity; if none does, the answer is `401`. If one could not decide (for example the webhook is unreachable) and no other accepted, the answer is `503`: an unavailable authenticator is never a pass, and never cached as a rejection.

| Type | What it does |
|---|---|
| `oidc` | Validates a JWT from one issuer: signature (keys from `jwks_url`, or the issuer's discovery document), `iss`, `aud` (required), `exp`, `nbf`, with `clock_skew` leeway. Keys are cached (`jwks_cache_ttl`), refreshed when an unknown key id appears, and the cached keys keep working if the issuer's key endpoint is down. A JWT from another issuer is not judged by this entry, so several issuers can share a chain. |
| `webhook` | Sends a `TokenReview` (`authentication.k8s.io/v1`) to `url`, for credentials SaFE cannot interpret itself (opaque tokens, session cookies). Answers are cached: accepted credentials for `cache_ttl` (default 30s), rejected ones for `cache_negative_ttl` (default 5s); concurrent requests with one credential share one review. |
| `builtin` | Hands the request to the built-in mode. Put it last to keep a local administrator able to sign in, including while the webhook is down. |

### Webhook protocol

Request (`POST`, JSON):

```json
{"apiVersion": "authentication.k8s.io/v1", "kind": "TokenReview",
 "spec": {"token": "<bearer token>", "cookie": "<session cookie>", "clientIP": "203.0.113.7", "userType": "<userType>"}}
```

`cookie`, `clientIP` and `userType` are extensions to the standard `spec`. Response (`200`):

```json
{"apiVersion": "authentication.k8s.io/v1", "kind": "TokenReview",
 "status": {"authenticated": true,
   "user": {"uid": "user-id", "username": "Alice", "groups": ["platform-admins"],
            "extra": {"email": ["alice@example.com"]}}}}
```

`uid` is the SaFE user id; the authenticator decides it. `authenticated: false` rejects the credential. Any other HTTP status, or a body that is not a `TokenReview`, means the review did not happen. Recognised `extra` keys: `email`, `name`, `actor`, `scopes`, `jti`, `exp` (unix seconds), `restricted` (`"true"` freezes the user for the request).

### stale_if_error

If the webhook cannot answer, a credential it **accepted** before keeps working for `stale_if_error` after its `cache_ttl` (default `30m`, set `-1` to turn it off), but never past its own `exp`. A credential it never accepted gets `503`. Revocation during an outage therefore takes effect after at most `cache_ttl + stale_if_error`. Each such request increments `safe_apiserver_auth_stale_served_total`, which is worth alerting on.

## Users and roles

* **Just-in-time users.** The first time an authenticated `uid` is seen, SaFE creates the user object (name is the `uid`, which must be a valid resource name; email and display name become annotations). Later requests only refresh those annotations. No role is written.
* **Groups map to roles at request time.** `auth.group_mappings` maps identity groups to SaFE roles; the result is computed for each request and never stored, so removing a user from a group takes effect on the next request. The effective roles are the user object's `roles` united with the mapped ones. The `workspace-admin` role in a mapping means managing the listed `workspaces`; any mapping with `workspaces` also grants access to them.

```yaml
auth:
  mode: external
  authenticators:
    - {type: oidc, issuer: "https://idp.example.com", audience: safe-apiserver}
    - {type: webhook, url: "https://authn.example.com/token-review"}
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
