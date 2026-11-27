# Team API

## Overview

Teams are owned by the identity service (`auth.mode: external`): it manages the teams and their members, and reports the teams of a user in the `teams` field of the introspection answer or in the JWT claim named by `teams_claim` (see [External Authentication](./external-authentication.md#teams)). SaFE stores only the bindings of teams to workspaces (`teams` of a workspace, see [Workspace API](./workspace.md#team-bindings)) and serves the display data of the identity service for the console. In builtin mode there are no teams: the lists are empty and bindings grant nothing.

The display data (names, member counts, managers, member lists) is read from the identity service with the client of the introspection authenticator, which needs the `identity.teams.read` scope, and is cached briefly. It is used for display only and never for authorization.

## API List

### 1. List Teams

List the teams of the identity service, for the team selector.

**Endpoint**: `GET /api/v1/teams`

**Authentication Required**: Yes

**Permission Requirements**: any signed-in user. A restricted (frozen) user gets 403. Not available to delegated tokens.

**Response Example**:

```json
{
  "totalCount": 1,
  "items": [
    {
      "id": "ml-infra",
      "displayName": "ML Infra",
      "description": "Training infrastructure",
      "memberCount": 12,
      "managers": [
        {"userId": "user-002", "email": "lisi@example.com", "name": "lisi", "preferredName": "lisi", "role": "manager"}
      ]
    }
  ]
}
```

**Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| totalCount | int | Number of teams |
| id | string | Team id |
| displayName | string | Team display name |
| description | string | Team description |
| memberCount | int | Number of team members |
| managers | []object | Managers of the team, with the fields of a team member (see "List Team Members") |

Items are sorted by id. The list is empty in builtin mode.

---

### 2. List Team Members

List the members of one team.

**Endpoint**: `GET /api/v1/teams/{TeamId}/members`

**Authentication Required**: Yes

**Path Parameters**:

| Parameter | Description |
|-----------|-------------|
| TeamId | Team id, a DNS-1123 label (400 otherwise) |

**Permission Requirements**: members of the team (the teams the caller's identity reports for the request) and system administrators; anyone else gets 403. Not available to delegated tokens.

**Response Example**:

```json
{
  "teamId": "ml-infra",
  "totalCount": 1,
  "items": [
    {"userId": "user-002", "email": "lisi@example.com", "name": "lisi", "preferredName": "lisi", "role": "manager"}
  ]
}
```

**Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| userId | string | User id |
| email | string | E-mail address |
| name | string | Display name |
| preferredName | string | Preferred username |
| role | string | Role of the user in the team, as the identity service reports it (`member` or `manager`) |

A user is a member when the team grants them a workspace on the request. When the identity service refuses the call (401 or 403) or sends an unreadable answer, the response is 502; when it fails with a 5xx status or cannot be reached, 503; when it does not know the team, 404. In builtin mode the list is empty.

---

### 3. Bind Team to Workspaces

Bind one team to several workspaces at once.

**Endpoint**: `PUT /api/v1/teams/{TeamId}/workspaces`

**Authentication Required**: Yes

**Request Parameters**:

```json
[
  {"workspace": "prod-cluster-ai-team", "role": "member"},
  {"workspace": "prod-cluster-dev-team", "role": "manager"}
]
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| workspace | string | Yes | Workspace ID |
| role | string | No | `member` (default) or `manager`. A team that is already bound gets this role |

At least one entry is required, and a workspace may be named only once (400 otherwise).

**Permission Requirements**: for each entry, the rules of "Update Workspace Teams" in the [Workspace API](./workspace.md#7-update-workspace-teams): update right on the workspace, and a system administrator for any change to a `manager` binding. Not available to delegated tokens.

Every entry is checked before anything is written, so a refused entry leaves all workspaces unchanged. When writing an entry fails, the bindings this request wrote are restored where no other request changed them since, and the error is returned. When something cannot be restored, the error message ends with "the team <id> stayed bound to: <workspace> (<role>), ...".

**Response Example**:

```json
{
  "teamId": "ml-infra",
  "totalCount": 2,
  "items": [
    {"workspaceId": "prod-cluster-ai-team", "workspaceName": "ai-team", "role": "member"},
    {"workspaceId": "prod-cluster-dev-team", "workspaceName": "dev-team", "role": "manager"}
  ]
}
```

---

### 4. List Team Workspaces

List the workspaces a team is bound to.

**Endpoint**: `GET /api/v1/teams/{TeamId}/workspaces`

**Authentication Required**: Yes

**Permission Requirements**: any signed-in user. Only the workspaces the caller may list are returned (all of them for a system administrator).

**Response**: the shape of "Bind Team to Workspaces", with the role of the team in each workspace, sorted by workspace ID.
