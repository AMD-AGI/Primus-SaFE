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
| ssoEnable | bool | Whether Single Sign-On (SSO) via OIDC is enabled. |
| ssoAuthUrl | string | Authorization URL for SSO login; present only if `ssoEnable` is true. |

#### Notes

- `ssoAuthUrl` is included only when SSO is enabled and properly configured.
- The values are derived from server-side configuration; they are read-only from this API.

