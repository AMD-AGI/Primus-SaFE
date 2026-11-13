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
* **The platform roles come only from the identity.** `system-admin` and `system-admin-readonly` are held by a request if and only if a group mapping grants them. The roles stored on the user object are ignored for these two, so an administrator whose group is removed at the identity provider stops being one when the credential is next authenticated afresh, even though the object still lists the role, and restoring the group restores the role. For an `introspection` credential that is within `cache_ttl` (30s by default). An `oidc` JWT carries its groups claim fixed until it expires, so a revoked administrator keeps the role until the token's `exp`; keep access tokens short-lived, or use `introspection` where revocation must take effect promptly. Nothing is written to the user object, so `auth.mode: builtin` finds it as it was. Every other role, the workspaces and the restriction are the user object's united with the mapped ones. A user who holds no platform role is an ordinary user and holds the base roles: the `default` role, unless the deployment sets `users.base_roles` (for example to `[self-read]`, a role that lets a user read only their own record, so that a new user holds no access until an administrator grants it). This applies to every request in external mode, including one the chain hands to the `builtin` entry: no identity provider vouched for it, so the platform roles stored on its user object do not count. There is no local-account exception; to sign in with local accounts, use `auth.mode: builtin`.
* **The administrator path is the identity provider.** In external mode an administrator is a user whose identity-provider credential carries a group that a mapping grants `system-admin`, and signs in through the identity provider. A built-in credential (a session or API key that a local account already holds) is an ordinary user, so a local administrator gets `403` on the administrator pages and `POST /auth/verify` returns no platform role for it.
* **`POST /auth/verify` runs the same chain.** The services behind the apiserver that authorize through it send the credential in the request body (a session cookie, an API key or a bearer token), and the endpoint authenticates it with `auth.authenticators`, in order, exactly as the apiserver does for a request: an identity provider's credential is answered with the user's roles as the apiserver computes them (the mapped `system-admin` included), a credential the chain rejects gets `401`, an authenticator that could not decide gets `503`, and a credential the chain hands to the `builtin` entry is answered as above, with no stored platform role. When a key is asked for (`includePlatformKey`, `includeVirtualKey`), the gateway virtual key is looked up by the email on the subject's own user record, for every credential the chain accepts and not by the `email` claim of the credential: for a local account (the record is never rewritten by the provider) a credential whose claim names another address cannot reach another user's gateway key; for an account the provider created, the record follows the provider's claim on every request, so the provider is trusted for the pairing of a subject and its email, as it is for the subject itself. No key is issued for a restricted user (see [`auth.key_actors`](#authkey_actors)). A delegated token (one with an `act` claim) is refused with `403`, as for the other credential routes, unless its actor is named in `auth.key_actors` (see [Delegated tokens](#delegated-tokens)). The endpoint is legacy: services should move to `GET /users/self` with the user's own credential, which returns the same roles and needs no internal token.

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

* **A user whose roles grant nothing but reading the user's own record is served nothing else.** Every route that serves a user starts with one check, before its handler runs: if the user's roles grant no more than `get` on the user's own `user` record (the built-in `self-read` role is exactly that), only `GET /users/{id}`, `GET /users/self/workspaces` and `POST /logout` are served, and every other route answers `403` with the error code `Primus.00051` and the message "The user has no access yet. Ask an administrator to grant it" (a different code from the `403` of a role that merely does not grant one thing, so a client can tell the two apart). It reads the rules of the roles, not their names, and a user whose roles cannot be found holds nothing (a system administrator is the exception: that user holds access whatever the Role objects say, so a missing platform Role does not lock the administrators out). The handlers' own checks are unchanged and still apply to everyone else.
* **`POST /auth/verify` returns the roles.** The response's `roles` field lists the roles the user holds as the apiserver sees them (the base roles and the mapped roles included, always a list), so a service that wants to serve only users who hold a certain role can require one. The `noAccess` field is `true` when the user holds no access (the rule of the previous item: nothing but reading the user's own record, or a user whose record or roles cannot be read), so a service that serves only users who hold access refuses that user, as the apiserver does everywhere else. A system administrator, read-only or not, always holds access.

Mapped roles and workspaces are part of the user every handler resolves for the request (`AccessController.GetRequestUser`), so the access controller (`Authorize`, `AuthorizeSystemAdmin`, role lookup), the checks handlers make on the user directly (model visibility, workspace access, the sandbox environment Secret, reserved workspace labels), `GET /users/self` and `GET /users/self/workspaces` all agree. That user is a copy for the request; it is never written back to the user object. For the same reason a `PATCH /users/{id}` that edits the caller's own workspaces stores only the workspaces the caller's groups do not grant (the console's row lists the granted ones too, and sends them back with an edit): a granted workspace is left out of the stored list unless the record already holds it, so it ends with the group. The web shell is an HTTP request and keeps the roles and workspaces of its identity like any other. An SSH session is not an HTTP request: it is authenticated by public key, not by an identity provider, so it follows the same rule as a built-in credential: the platform roles stored on the user object do not count, and the user holds the `default` role and the workspaces and other roles of the record.

## Upgrading from a release that kept local administrators

**Breaking change for deployments that relied on a local administrator.** Earlier releases told operators to put `builtin` last in `auth.authenticators` to keep a local administrator able to sign in, including while the introspection endpoint is down. That exception is gone: in external mode the platform roles (`system-admin`, `system-admin-readonly`) held by a request come only from `auth.group_mappings`, and the roles stored on user objects, such as that of a local break-glass administrator, are not counted. After the upgrade such an account still authenticates through the `builtin` entry (its sessions and API keys keep working) but is an ordinary user, and `POST /auth/verify` and SSH sessions agree. A local administrator therefore cannot administer the platform through the console either: every administrator signs in through the identity provider. The services that authorize through `POST /auth/verify` now see the identity provider's administrators there (the endpoint runs the same chain as the apiserver) and no local one.

Before upgrading a deployment in external mode:

* Make sure every administrator is granted `system-admin` (or `system-admin-readonly`) by a group mapping at the identity provider. There is no local fallback while the identity provider is down.
* If a local administrator is needed as a break-glass path, that is `auth.mode: builtin`: set it back (a configuration change and an apiserver roll) to sign in with local accounts. User records created while external mode was on remain. Roles that SCIM wrote from `group_mappings`, including `system-admin`, are stored on those records and count again under builtin. A user SCIM deleted is gone. Review the User list before rolling back.

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

`GET /api/v1/envs` reports `authMode`, and the console hides the password form, registration, password change and role editing accordingly. In this mode `/envs` does not advertise the built-in sign-in either: `ssoEnable` is `false` and `ssoAuthUrl` is empty unless `auth.login_url` is set, in which case they are that URL.

**API keys.** The apiserver refuses to issue a key in this mode, so a user creates one at the identity provider, and the console needs a route to it. Nothing in the apiserver or the console guesses one: the deployment names it, and `/envs` reports it as `apiKeysUrl` (from `auth.api_keys_url`), a path on the console's own origin, such as `/api/v1/apikeys`. With `apiKeysUrl` the console creates keys there, and lists and deletes the provider's keys there; without it the console offers no key creation and says so. Two ways to provide the route:

* the chart's console does it: set `auth.api_keys_upstream` to the provider's service as `host[:port]` (the full in-cluster name, `service.namespace.svc.cluster.local:port`, because the console's resolver does not apply search domains). With `auth.mode: external` the console's nginx then proxies `/api/v1/apikeys` and what is below it, query string included, to that service, ahead of its route to the apiserver, and `apiKeysUrl` is `/api/v1/apikeys`. The chart's ingress sends the console's host to the console, so no other rule is needed;
* or a gateway in front of the console routes the path itself, and `auth.api_keys_url` is set to it. Do not set `auth.api_keys_url` without one of the two: the path then reaches the apiserver like the rest of `/api/v1/`, which refuses to issue the key, and the console would show a button that always fails.

Neither takes effect unless `auth.mode` is `external`. The console's usual `/api/apikeys` (its alias to the apiserver) is never routed to the provider. It is how the console lists and revokes the keys **this apiserver issued before the switch**: such a key is still accepted through the `builtin` entry, so it must stay possible to revoke it, and it is not in the provider's list. With `apiKeysUrl` set the console shows those keys in their own section, "Keys issued before the switch" (only while there are any), and deletes each key on the side it is listed from: the provider's at the provider, the apiserver's at the apiserver. Without `apiKeysUrl` the one list is the apiserver's, as before. The route and its shape (the provider's list and create answers use the apiserver's fields) were read from the provider's code and are not enforced here.

Because a local sign-in is not available, backing out of an outage of the provider is a configuration change, not a login: set `auth.mode: builtin` (see below).

## Delegated tokens

A token that carries an `act` claim (RFC 8693) means a party is acting on behalf of the user; SaFE records the actor, and restricts it, whoever issued it:

* **Audit.** `audit_logs` has `actor_client_id` and `token_jti`. Reads made with a delegated token are recorded too (without bodies), not only writes.
* **Capability exclusions.** A delegated token can never call API-key, Secret, user-management, SSH-key-writing, gateway-credential or token-verification routes, whatever its scope, with one exception for the last (see `auth.key_actors` below). The list is pinned by tests against the real route table; a new route whose path mentions a credential fails the test until it is classified.
* **Scope.** A delegated token needs `safe.api` (everything the user may do, minus the exclusions) or `safe.read` (reads only) in its `scope` claim; without one it is refused. A read is judged by what the request can do, not by its HTTP method: the web shell is a `GET` that upgrades into an interactive exec session, so a request that asks the server to switch protocols (an `Upgrade` header or a `Connection: upgrade` token, whichever route it goes to), and the routes that open a shell, exec, attach or port-forward session, need `safe.api`. A test reads the handlers that upgrade the connection and checks that their routes are classified.

A malformed `act` claim is rejected rather than read as a direct login.

### `auth.key_actors`

A service that acts for a user and needs the user's platform key or gateway virtual key can get them from `POST /auth/verify` without ever holding the user's own credential, if it is named in `auth.key_actors`: a list of client ids, exact matches, empty by default (then a delegated token is refused there with `403`, as before).

```
auth:
  key_actors: [client-a]
```

The service sends a delegated token in the request body (`authorization: "Bearer <token>"`, with `includePlatformKey` and/or `includeVirtualKey`) and the internal token header, as any caller of the endpoint. The token is accepted only when all of this holds, and refused with `403` otherwise:

* an authenticator of the chain accepts it, which includes its issuer, expiry and **audience** (the `audience` the authenticator requires, for example one that names this apiserver); a token for another audience is `401`, as for any request;
* it carries an `act` claim whose actor is a **client id** named in `auth.key_actors`: `act.client_id`, or `act.sub` when the token's own `client_id` claim (RFC 9068; for an introspected answer, its `client_id`, RFC 7662) is the same id, which is how an identity provider that states the client only in `act.sub` identifies it. An `act.sub` alone, or one that differs from the token's `client_id`, is a subject in the issuer's namespace, which can be a user: it is never compared with the list (it still names the actor in the audit log, as `act.client_id`, else `act.sub`; a token whose two members differ is the `client_id`'s). The JWT and the introspection authenticators read it the same way, and the match is exact;
* its `scope` contains `safe.api`; `safe.read` alone is not enough.

The answer is about the token's subject, and about nobody else: the keys are those of the user the chain authenticated, the request has no field that names another user, and a field that does is ignored. The `type` of the answer is `delegated`; the roles are the subject's, as a request of the subject sees them. The fields of the answer are those of any `POST /auth/verify` answer (`id`, `name`, `email`, `preferredName`, `exp`, `type`, `roles`, and the keys asked for); a key is returned only when its flag is sent.

**What the listed client receives is the user's credentials, not a session of limited reach.** The `platformKey` is the user's ordinary, stable API key of this apiserver, minted on the first request if the user has none. It is not a delegated credential: whoever holds it can call every route a delegated token is excluded from (API keys, Secrets, user management, SSH keys, gateway credentials, token verification) and everything else the user may do, with no `safe.api`/`safe.read` limit (the scope of the delegated token only limits the exchange for the key, not what the key does), and for as long as the key is valid, not as long as the delegated token. Calls made with it are audited as the user's own, without `actor_client_id` or `token_jti`. The `virtualKey` is the user's gateway virtual key, with the user's gateway budget. So naming a client in `auth.key_actors` is **equivalent to full impersonation** of any user it can get a delegated token for, for the life of those keys: only the capability exclusions that apply to the delegated token itself stay in force, and the route this opens is the endpoint, not the reach of what it returns. Keys are not issued per actor, so a later call with a key cannot be traced to the actor by the key; what ties the issue to the actor is the log line written for every answer that carries a key (`keys issued to a delegated credential: actor, user, jti`), next to the one written when the token is accepted.

**No keys for a restricted user.** When the identity is restricted (the provider's `restricted` claim, or a restricted/frozen user record) and a key is asked for, the endpoint answers `403` (`the user is restricted`) and issues nothing, and counts it as `restricted` in `safe_apiserver_auth_failures_total`. Every request the apiserver authenticates through the chain is refused for such a user, but the keys this endpoint returns are authenticated through the `builtin` entry, which has no such claim to apply, so a key issued to a restricted user would keep working while the provider has frozen the user. This holds for every credential the chain accepts (a user's own token as much as a delegated one), not only for key actors; a credential the chain hands to the `builtin` entry is judged there, as before. A key issued before the user was restricted is not revoked by the claim: restrict the user record too, or delete the key, if that must take effect at once.

Naming a client opens this one endpoint and nothing else: every other route a delegated token cannot call (above) stays refused, `POST /auth/verify` included when it is sent through the request path of the apiserver rather than to the endpoint itself. Each accepted call is logged with the actor, the user and the token id, and refusals are counted in `safe_apiserver_auth_failures_total` as `delegated_forbidden` (actor not listed, or not a client id), `delegated_scope`, or `restricted`.

The list is for operators to reason about: a listed client can obtain the platform key of any user it can get a delegated token for, so list only clients whose token issuance the identity provider limits to users that have authorized them. The endpoint is legacy (see above); `key_actors` goes with it.

#### In builtin mode

`auth.key_actors` also works with `auth.mode: builtin`, for this one endpoint, when `auth.authenticators` holds at least one `oidc` authenticator (its issuer, JWKS and required `audience`). Nothing else of the external mode is turned on: requests are still authenticated by the built-in mode only, logins, sessions and API keys are unchanged, and no group mapping applies. `POST /auth/verify` then answers a bearer JWT that one of those `oidc` authenticators accepts (issuer signature, expiry, audience), whose actor is a listed client id read as above, and whose `scope` contains `safe.api`, with the keys of its subject, which must be an existing user: builtin mode creates no user, and a subject that is not one is `401`. The `name`, `email` and roles of the answer are the user record's, never the token's claims, so the gateway virtual key is looked up by the record's email. A restricted user gets no key (`403`). Every other credential, including a token of the issuer that is not delegated or that no authenticator accepts, goes to the built-in path exactly as before. Only `oidc` entries that take the user id from `sub` (the default `id_claim`) are used; `introspection` and `builtin` entries, and an `oidc` entry with another `id_claim`, are ignored in this mode, so the endpoint never asks an introspection server about the apiserver's own sessions. Either list empty: the endpoint is as it always was.

```yaml
auth:
  mode: builtin
  authenticators:
    - {type: oidc, issuer: "https://idp.example.com", audience: primus-safe, jwks_url: "http://idp.idp.svc/.well-known/jwks.json"}
  key_actors: [client-a]
```

## SCIM

When `auth.mode` is `external` and `auth.scim.enabled` is `true`, the apiserver serves SCIM 2.0 under `/scim/v2/Users` for the identity service. The routes are not behind user authentication. The caller sends `Authorization: Bearer <token>`, compared in constant time with the token in Secret `auth.scim.token_secret` (default `primus-safe-scim-token`, namespace `primus-safe`). The apiserver creates that Secret, holding 32 random bytes, when it is absent. The apiserver's service account already may create Secrets. If `auth.scim.enabled` is set and `auth.mode` is not `external`, the endpoint is not served.

Responses use `Content-Type: application/scim+json`. Errors use the SCIM error schema (`schemas`, `status`, `detail`).

| Request | Behaviour |
|---|---|
| `GET /scim/v2/Users?startIndex=&count=` | ListResponse. `startIndex` is 1-based (below 1 counts as 1). `count` defaults to 100, a negative value counts as 0, and values above 1000 are capped at 1000. `active` is false when `restrictedType` is frozen. `id` and `externalId` are the user id. |
| `GET /scim/v2/Users/{id}` | That user. An unknown id is 404. |
| `PATCH /scim/v2/Users/{id}` | Replaces `active`. `false` sets `restrictedType` to frozen and annotation `primus-safe.scim.deactivated` to `true`. `true` clears the freeze only when that annotation is present, then removes it. The body is a PatchOp (`Operations` of `{"op":"replace","path":"active","value":false}`) or `{"op":"replace","value":{"active":false}}`. |
| `DELETE /scim/v2/Users/{id}` | Removes the user from workspaces the user manages, then deletes the user (the same cleanup as `DELETE /api/v1/users/{id}`). An unknown id is 204. |

The apiserver registers the endpoint in the background, retrying with backoff, with `PUT {identity}/oauth/scim-target` and HTTP Basic. The client id and secret are the introspection authenticator's `client_id` and `client_secret_file`. The body is `{"url":"<auth.scim.public_url>","bearerToken":"<token>"}`. `auth.scim.public_url` is the base the identity service calls; it appends `/Users/{id}`. The identity root is `auth.scim.identity_url` when set. Otherwise it is taken from the introspection URL or issuer by dropping a `/oauth/...` suffix. A URL that is neither a host root nor under `/oauth/` is not derived; set `auth.scim.identity_url`.

`POST /api/v1/users` and `DELETE /api/v1/users/{id}` are registered only when `auth.mode` is `builtin`. In external mode, whether or not SCIM is enabled, a patch that sets `restrictedType` to frozen sends `PUT /api/v1/identities/{id}/status` with `{"status":"disabled"}`; a patch that clears a freeze sends `{"status":"active"}`. The call uses the caller's bearer credential. If the identity service cannot be reached, or its base URL is not configured, the answer is 503. If it refuses the call, the answer is 502. The local user is left unchanged in both cases. Builtin mode does not call the identity service.

`bootstrap/upgrade.sh` carries `auth_scim_enabled` (`true` or `false`), `auth_scim_identity_url`, `auth_scim_public_url` and `auth_scim_token_secret` with the other `auth_*` keys.

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
auth_scim_enabled=true
auth_scim_identity_url=https://idp.example.com
auth_scim_public_url=https://console.example.com/scim/v2
auth_scim_token_secret=primus-safe-scim-token
```

They set `auth.mode`, `auth.login_url`, `auth.client_secret` (the name of the Secret in the release namespace that holds the introspection client secret, not the secret itself), `auth.authenticators`, `auth.group_mappings`, `auth.key_actors`, and the SCIM settings `auth.scim.enabled`, `auth.scim.identity_url`, `auth.scim.public_url` and `auth.scim.token_secret` (`auth_scim_enabled` is `true` or `false`). Values that are not usable (a JSON key that is not an array, a mode that is neither `builtin` nor `external`, `external` without authenticators, without `login_url`, without a group granted `system-admin`, without an introspection or builtin authenticator unless `auth.allow_oidc_only` is true, or with SCIM enabled and no resolvable identity URL) stop the upgrade before it changes anything; the script needs `jq` when `.env` sets any of them (`auth_mode=builtin` on its own excepted) and to upgrade a release that is already external when `.env` sets `auth_mode=external`; a release that is not external upgrades without it.

An introspection authenticator sets `allow_missing_audience: true`. An API key's introspection answer names no audience.

`auth.login_url` is required when `auth.mode` is `external`. The chart does not derive it. `auth.provider_client` is off by default: the apiserver serves SCIM and does not call the identity service to register the target or to forward a status change. `auth.connection`, `auth.accounts_url` and `auth.keys_console_url` are what `GET /envs` returns for the console. An empty connection omits that parameter on the authorize request.

`rbac.subject_prefix` is the same value a consumer sets as `RBAC_SUBJECT_PREFIX`. Builtin mode projects platform administrators as Users and writes no Groups. External mode binds platform roles only to the Groups `<prefix>system-admin` and `<prefix>system-admin-readonly`, so the mapped group names are those names. Sandbox admission and a SubjectAccessReview of a sandbox user need external mode and those mappings. `auth.group_mappings` is read when resource-manager starts. The SCIM server accepts a replace of `groups`; RFC 7643 defines that attribute as read-only, and this replace is an extension.

Omitting `auth_mode` on a release that is not external renders the chart default `builtin`. Omitting it on a release that is already `external` is refused. Setting `auth_mode=external` refuses an upgrade that would drop another key the release has: the script exits non-zero before it applies anything and names the missing keys; copy them from `helm -n primus-safe get values primus-safe -o json | jq .auth`. With `auth_mode=external` it also refuses when it cannot read the running release.

The client Secret is created in the release namespace. The chart mounts that Secret and does not read another namespace.

## Turning external authentication on

1. The identity provider is serving discovery, token, introspection and, when SCIM is enabled, the SCIM client.
2. The client Secret exists in the SaFE namespace.
3. The SCIM target is configured on the provider. `auth.provider_client` stays off unless this deployment should register that target itself.
4. The database job has finished.
5. `auth.authenticators` is `oidc`, then `introspection`, then `builtin`. When `sso.login.enable` is true, one `oidc` entry's `audience` is `sso.login.client_id`. The console sign-in cookie is that client's ID token, and the chain rejects it otherwise.
6. Sign-in, `GET /users/self`, API keys and the LLM proxy succeed.
7. For sandbox admission, deploy this release with `sandbox.admission` enabled (the example overlay, plus `actor` and the key set) and check that a non-member is denied. Then move the gatekeeper-constraints addon to 0.1.5. That addon no longer checks membership.

## Rolling back

Set `auth.mode: builtin` and roll the apiserver. The endpoints listed under "What the apiserver stops offering" work again. User records that external mode created stay. `spec.roles` that a SCIM PATCH wrote from `group_mappings` stay on the record and are effective again, including platform roles. A user that SCIM deleted is not recreated. Review those records after the rollback.

With `bootstrap/upgrade.sh` or the CD job, `auth_mode=builtin` in `.env` renders builtin. Leaving `auth_mode` unset on an external release is refused. To run external again, set `auth_mode=external` and the other `auth_*` keys.
