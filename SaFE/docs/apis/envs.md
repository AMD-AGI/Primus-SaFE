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
| authMode | string | How requests are authenticated: `builtin` (the default) or `external`. With `external`, an identity provider owns sign-in, accounts, API keys and platform roles, and the console hides the password form, registration, password change and role editing, and sends API key creation, listing and deletion to the identity provider's `/api/v1/apikeys` route (see [External Authentication](external-authentication.md)). An apiserver that predates this field omits it, which means `builtin`. |
| ssoEnable | bool | Whether Single Sign-On (SSO) via OIDC is enabled. With `authMode` `external` it is `true` only when `auth.login_url` is set: the built-in sign-in is refused there and is not advertised. |
| ssoAuthUrl | string | Authorization URL for SSO login; present only if `ssoEnable` is true. With `authMode` `external` it is `auth.login_url`. |

#### Notes

- `ssoAuthUrl` is included only when SSO is enabled and properly configured.
- The values are derived from server-side configuration; they are read-only from this API.

