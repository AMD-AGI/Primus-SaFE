# User Notification Settings API

Per-user email notification configuration. When enabled, the user receives email notifications on workload status changes (Running, Succeeded, Failed, Stopped).

Default: **off** (no notifications unless explicitly enabled).

---

## GET /api/v1/users/:name/settings

Retrieve the notification settings for a user.

### Path Parameters

| Parameter | Type   | Required | Description                              |
|-----------|--------|----------|------------------------------------------|
| name      | string | Yes      | User ID, or `self` for the current user  |

### Authentication

Cookie / `Authorization: Bearer <api-key>`

### Response

**200 OK**

```json
{
  "enableNotification": false
}
```

| Field              | Type    | Description                                    |
|--------------------|---------|------------------------------------------------|
| enableNotification | boolean | Whether email notification is enabled for this user |

### Example

```bash
# Query own settings
curl -s http://<host>/api/v1/users/self/settings \
  -H "Authorization: Bearer <api-key>"

# Admin queries another user
curl -s http://<host>/api/v1/users/<user-id>/settings \
  -H "Authorization: Bearer <api-key>"
```

---

## PUT /api/v1/users/:name/settings

Update the notification settings for a user. Regular users can only update their own settings; administrators can update any user.

### Path Parameters

| Parameter | Type   | Required | Description                              |
|-----------|--------|----------|------------------------------------------|
| name      | string | Yes      | User ID, or `self` for the current user  |

### Authentication

Cookie / `Authorization: Bearer <api-key>`

### Request Body

```json
{
  "enableNotification": true
}
```

| Field              | Type    | Required | Description                                      |
|--------------------|---------|----------|--------------------------------------------------|
| enableNotification | boolean | No       | `true` to enable email notifications, `false` to disable |

### Response

**200 OK** (empty body)

### Error Responses

| Status | errorCode    | Description                    |
|--------|-------------|--------------------------------|
| 400    | BadRequest  | Invalid request body           |
| 401    | Unauthorized| Authentication required        |
| 403    | Forbidden   | No permission to update target user |
| 404    | NotFound    | User not found                 |

### Example

```bash
# Enable notifications
curl -s -X PUT http://<host>/api/v1/users/self/settings \
  -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/json" \
  -d '{"enableNotification": true}'

# Disable notifications
curl -s -X PUT http://<host>/api/v1/users/self/settings \
  -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/json" \
  -d '{"enableNotification": false}'
```

---

## Behavior

- When `enableNotification` is `true`, all workloads owned by this user will trigger email notifications on phase transitions: **Running**, **Succeeded**, **Failed**, **Stopped**.
- When `enableNotification` is `false` (default), no email notifications are sent for this user's workloads.
- The setting is stored as an annotation (`primus-safe.user.enable.notification`) on the User CR and takes effect immediately.
- No database migration is required.
