## Envs API

### Overview
Returns environment configuration flags and endpoints used by the frontend and clients. This endpoint aggregates settings from the backend configuration.

### Get Environment Settings

- Endpoint: `GET /api/v1/envs`
- Authentication Required: No (public)

#### Response Example

```json
{
  "enableLogDownload": true,
  "enableLog": true,
  "enableSsh": true,
  "sshPort": 30122,
  "sshIP": "10.0.0.10",
  "authMode": "builtin",
  "ssoEnable": true,
  "ssoAuthUrl": "https://accounts.example.com/oauth2/authorize?client_id=abc&redirect_uri=http%3A%2F%2Ftest.primus-safe.amd.com&response_type=code&scope=openid+profile+email+groups"
}
```

#### Field Description

| Field | Type | Description |
|-------|------|-------------|
| enableLogDownload | bool | Whether log download is enabled (requires S3 support). |
| enableLog | bool | Whether logging features are enabled. |
| enableSsh | bool | Whether SSH features (including WebShell) are enabled. |
| sshPort | int | SSH service port exposed by the system. |
| sshIP | string | SSH service IP address. |
| authMode | string | How requests are authenticated: `builtin` (the default) or `external`. With `external`, an identity provider owns sign-in, accounts, API keys and platform roles, and the console hides the password form, registration, password change and role editing (see [External Authentication](external-authentication.md)). An apiserver that predates this field omits it, which means `builtin`. |
| apiKeysUrl | string | With `authMode` `external` only, and only when the deployment routes it (`auth.api_keys_url`, or `auth.api_keys_upstream` in the chart): the path on the console's own origin where API key creation, listing and revocation reach the identity provider, for example `/api/v1/apikeys`. The console creates keys there, and lists and revokes the provider's keys there, and lists the keys this apiserver issued before the switch from `/apikeys` beside them. Absent otherwise, and then the console offers no key creation: the apiserver refuses to issue keys in external mode. Only a path is reported; a value that is a URL, has a query or does not start with a single `/` is treated as not set. |
| ssoEnable | bool | Whether Single Sign-On (SSO) via OIDC is enabled. With `authMode` `external` it is `true` only when `auth.login_url` is set: the built-in sign-in is refused there and is not advertised. |
| ssoAuthUrl | string | Authorization URL for SSO login; present only if `ssoEnable` is true. With `authMode` `external` it is `auth.login_url`. Otherwise it is the URL of the provider of the `sso` Secret, whose codes `POST /api/v1/login` redeems without a code verifier (also for the web apps that bridge through the console); empty when that provider is not configured. |
| ssoLoginUrl | string | With `sso.login` enabled (`authMode` `builtin`): the console's sign-in, the authorization endpoint of that provider for the console's client id and callback, asking for PKCE (`code_challenge_method=S256`). The console uses it instead of `ssoAuthUrl`: it adds its `state` and an S256 `code_challenge`, keeps the verifier in the tab, and sends it as `code_verifier` with the code to `POST /api/v1/login`. Absent otherwise. |
| ssoPkce | bool | `true` whenever `sso.login` is enabled, also while its provider cannot be reached and `ssoLoginUrl` is therefore absent: the console then offers no sign-in rather than falling back to `ssoAuthUrl`, and uses a code at its callback only with the PKCE verifier it kept. Absent otherwise. |

#### Notes

- `ssoAuthUrl` is included only when SSO is enabled and properly configured.
- The values are derived from server-side configuration; they are read-only from this API.

