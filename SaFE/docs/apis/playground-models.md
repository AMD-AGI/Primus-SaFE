# Playground Models API

## Overview

Playground Models API provides a model management interface for the AI Playground. Users can add models from Hugging Face or other sources, configure download targets, and start inference services. The system automatically fetches model metadata (display name, description, icon, tags) from Hugging Face when available.

## API List

### 1. Create Playground Model

**Endpoint**: `POST /api/v1/playground/models`

**Authentication Required**: Yes

**Request Example (Remote API - OpenAI/DeepSeek)**:
```json
{
  "displayName": "DeepSeek Chat",
  "description": "DeepSeek AI's conversational model with strong reasoning capabilities",
  "icon": "https://example.com/deepseek-icon.png",
  "label": "DeepSeek",
  "tags": ["chat", "reasoning", "api"],
  "source": {
    "url": "https://api.deepseek.com/v1",
    "accessMode": "remote_api",
    "token": "sk-xxxxxxxxxxxxxxxxxxxxx"
  }
}
```

**Request Example (Hugging Face - Local Deployment)**:
```json
{
  "source": {
    "url": "https://huggingface.co/Qwen/Qwen2.5-7B-Instruct",
    "accessMode": "local",
    "token": "hf_xxxxxxxxxxxxxxxxxxxxx"
  },
  "downloadTarget": {
    "type": "local",
    "localPath": "/data/models/qwen2.5-7b"
  },
  "resources": {
    "cpu": "16",
    "memory": "64Gi",
    "gpu": "2"
  }
}
```

**Request Example (S3 Download Target)**:
```json
{
  "source": {
    "url": "meta-llama/Llama-3.1-8B-Instruct",
    "accessMode": "local",
    "token": "hf_xxxxxxxxxxxxxxxxxxxxx"
  },
  "downloadTarget": {
    "type": "s3",
    "s3Config": {
      "endpoint": "https://s3.amazonaws.com",
      "bucket": "my-models-bucket",
      "region": "us-west-2",
      "accessKeyID": "AKIAIOSFODNN7EXAMPLE",
      "secretAccessKey": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
    }
  },
  "resources": {
    "cpu": "8",
    "memory": "32Gi",
    "gpu": "1"
  }
}
```

**Request Body Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| displayName | string | Conditionally | Model display name (required for remote_api, auto-filled for local) |
| description | string | No | Model description (optional for remote_api, auto-filled for local) |
| icon | string | No | Model icon URL (optional for remote_api, auto-filled for local) |
| label | string | No | Model author/organization (optional for remote_api, auto-filled for local) |
| tags | []string | No | Model tags (optional for remote_api, auto-filled for local) |
| source | object | Yes | Model source configuration |
| source.url | string | Yes | Model URL: Hugging Face URL for "local" mode, API endpoint for "remote_api" mode |
| source.accessMode | string | Yes | Access mode: "remote_api" (call API directly) or "local" (download and deploy) |
| source.token | string | No | Token for authentication (Hugging Face token or API key) |
| downloadTarget | object | Conditionally | Required if accessMode is "local"; defines where to store the model |
| downloadTarget.type | string | Conditionally | Storage type: "local" or "s3" |
| downloadTarget.localPath | string | Conditionally | Host path for local storage (only for type "local") |
| downloadTarget.s3Config | object | Conditionally | S3 configuration (only for type "s3") |
| downloadTarget.s3Config.endpoint | string | Conditionally | S3 endpoint URL |
| downloadTarget.s3Config.bucket | string | Conditionally | S3 bucket name |
| downloadTarget.s3Config.region | string | Conditionally | S3 region |
| downloadTarget.s3Config.accessKeyID | string | Conditionally | S3 access key ID |
| downloadTarget.s3Config.secretAccessKey | string | Conditionally | S3 secret access key |
| resources | object | No | Resource requirements for inference service |
| resources.cpu | string | No | CPU cores (e.g., "8", "16") |
| resources.memory | string | No | Memory size (e.g., "32Gi", "64Gi") |
| resources.gpu | string | No | GPU count (e.g., "1", "2") |

**Notes**:
- **For `accessMode: "local"`**:
    - Only Hugging Face URLs are supported
    - Metadata fields (displayName, description, icon, label, tags) are automatically fetched from Hugging Face
    - A download job is created to pull the model files
    - `downloadTarget` is required to specify storage location
- **For `accessMode: "remote_api"`**:
    - `displayName` is required (user must provide)
    - Other metadata fields (description, icon, label, tags) are optional
    - No download occurs; the model is accessed via external API
    - Supports any API endpoint (OpenAI, DeepSeek, Claude, etc.)
- The token is securely stored in a Kubernetes Secret and never exposed in the CRD or database.

**Response**:
```json
{
  "id": "model-a3k9x"
}
```

---

### 2. List Playground Models

**Endpoint**: `GET /api/v1/playground/models`

**Authentication Required**: Yes

**Query Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| limit | int | No | Records per page, default 10 |
| offset | int | No | Pagination offset, default 0 |
| inferenceStatus | string | No | Filter by inference status (e.g., "Running", "Pending", "Stopped") |
| accessMode | string | No | Filter by access mode: "remote_api" or "local" |

**Response Example**:
```json
{
  "total": 25,
  "items": [
    {
      "id": "model-a3k9x",
      "displayName": "DeepSeek OCR",
      "description": "Inference using Huggingface transformers on NVIDIA GPUs",
      "icon": "https://cdn-avatars.huggingface.co/v1/production/uploads/...",
      "label": "deepseek-ai",
      "tags": "deepseek,vision-language,ocr",
      "version": "",
      "sourceURL": "deepseek-ai/DeepSeek-OCR",
      "accessMode": "remote_api",
      "sourceToken": "model-a3k9x",
      "downloadType": "",
      "localPath": "",
      "s3Config": "",
      "cpu": "",
      "memory": "",
      "gpu": "",
      "phase": "Ready",
      "message": "Model ready (AccessMode: remote_api)",
      "inferenceID": "inf-model-a3k9x-xyz123",
      "inferencePhase": "Running",
      "createdAt": "2025-11-25T10:30:00Z",
      "updatedAt": "2025-11-25T10:35:00Z",
      "deletionTime": null,
      "isDeleted": false
    }
  ]
}
```

**Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| total | int64 | Total number of models matching the filter (not limited by pagination) |
| items | array | Array of model objects |
| id | string | Model unique ID |
| displayName | string | Model display name |
| description | string | Model description |
| icon | string | Model icon URL |
| label | string | Model author/organization |
| tags | string | Comma-separated tags |
| version | string | Model version |
| sourceURL | string | Model source URL |
| accessMode | string | Access mode: "remote_api" or "local" |
| sourceToken | string | Name of the Kubernetes Secret storing the token |
| downloadType | string | Download target type: "local" or "s3" |
| localPath | string | Local storage path (if applicable) |
| s3Config | string | S3 configuration JSON (if applicable) |
| cpu | string | CPU resource allocation |
| memory | string | Memory resource allocation |
| gpu | string | GPU resource allocation |
| phase | string | Model status: "Pending", "Pulling", "Ready", "Failed" |
| message | string | Status message |
| inferenceID | string | Associated inference service ID (empty if not started) |
| inferencePhase | string | Inference service status (empty if not started) |
| createdAt | string | Creation timestamp (RFC3339) |
| updatedAt | string | Last update timestamp (RFC3339) |
| deletionTime | string | Deletion timestamp (null if not deleted) |
| isDeleted | bool | Soft delete flag |

---

### 3. Get Playground Model Details

**Endpoint**: `GET /api/v1/playground/models/{id}`

**Authentication Required**: Yes

**Response Example**:
```json
{
  "id": "model-a3k9x",
  "displayName": "Qwen2.5-7B-Instruct",
  "description": "Qwen2.5 is a large language model series...",
  "icon": "https://cdn-avatars.huggingface.co/v1/production/uploads/...",
  "label": "Qwen",
  "tags": "text-generation,conversational,llm",
  "version": "v1.0",
  "sourceURL": "Qwen/Qwen2.5-7B-Instruct",
  "accessMode": "local",
  "sourceToken": "model-a3k9x",
  "downloadType": "local",
  "localPath": "/data/models/qwen2.5-7b",
  "s3Config": "",
  "cpu": "16",
  "memory": "64Gi",
  "gpu": "2",
  "phase": "Ready",
  "message": "Download completed successfully",
  "inferenceID": "",
  "inferencePhase": "",
  "createdAt": "2025-11-25T09:00:00Z",
  "updatedAt": "2025-11-25T09:15:00Z",
  "deletionTime": null,
  "isDeleted": false
}
```

**Field Description**: Same as in "List Playground Models" response.

---

### 4. Delete Playground Model

**Endpoint**: `DELETE /api/v1/playground/models/{id}`

**Authentication Required**: Yes

**Description**:
Deletes a playground model. This operation will:
- Delete the Model custom resource from Kubernetes
- Delete the associated token Secret (if exists)
- Soft-delete the database record (sets `is_deleted = true` and `deletion_time`)
- Cascade delete any download Jobs via OwnerReference

**Response**:
```json
{
  "message": "model deleted successfully",
  "id": "model-a3k9x"
}
```

---

### 5. Toggle Playground Model (Start/Stop Inference)

**Endpoint**: `POST /api/v1/playground/models/{id}/toggle`

**Authentication Required**: Yes

**Description**:
Starts or stops an inference service for the model. This creates or deletes an Inference custom resource.

**Request Body**:
```json
{
  "enabled": true
}
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| enabled | bool | Yes | `true` to start inference, `false` to stop |

**Response (Start)**:
```json
{
  "message": "inference started",
  "inferenceId": "inf-model-a3k9x-xyz123"
}
```

**Response (Stop)**:
```json
{
  "message": "inference stopped"
}
```

**Response (Already Running)**:
```json
{
  "message": "inference already exists",
  "inferenceId": "inf-model-a3k9x-xyz123"
}
```

**Notes**:
- Each model can have only one active inference service at a time.
- The inference service type (ModelForm) is automatically determined by the model's accessMode:
    - `accessMode: "remote_api"` → `ModelForm: "API"` (proxy to external API)
    - `accessMode: "local"` → `ModelForm: "ModelSquare"` (local deployment)
- When starting an inference, the Model's `status.inferenceID` is updated with the new inference ID.
- When stopping an inference, the Inference CR is deleted and the Model's `status.inferenceID` is cleared.

---

## Model Lifecycle

### Phase Transitions

```
Create Model (POST /playground/models)
    ↓
[Pending] → Controller creates download Job (if accessMode = "local")
    ↓
[Pulling] → Download in progress (Job running)
    ↓
[Ready] → Model available for use
    or
[Failed] → Download failed

Toggle ON (POST /playground/models/:id/toggle with enabled=true)
    ↓
Create Inference CR
    ↓
Model.Status.InferenceID updated
    ↓
Inference Controller creates Workload
    ↓
[Running] → Inference service available

Toggle OFF (POST /playground/models/:id/toggle with enabled=false)
    ↓
Delete Inference CR
    ↓
Model.Status.InferenceID cleared
```

### Access Modes

| Access Mode | Description | Download Required | Inference Type |
|-------------|-------------|-------------------|----------------|
| `remote_api` | Call external API directly (e.g., OpenAI, DeepSeek API) | No | API proxy |
| `local` | Download model files and deploy inference service locally | Yes | Local GPU deployment |

---

## Error Handling

### Common Error Responses

**400 Bad Request**:
```json
{
  "error": "invalid request body: ...",
  "code": 400
}
```

**401 Unauthorized**:
```json
{
  "error": "user not authenticated",
  "code": 401
}
```

**404 Not Found**:
```json
{
  "error": "playground model not found: model-xyz",
  "code": 404
}
```

**500 Internal Server Error**:
```json
{
  "error": "failed to create model resource: ...",
  "code": 500
}
```

---

## Examples

### Example 1: Add a Remote API Model (DeepSeek API)

```bash
curl -X POST http://localhost:8080/api/v1/playground/models \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{
    "displayName": "DeepSeek Chat",
    "description": "DeepSeek AI conversational model",
    "icon": "https://example.com/deepseek-icon.png",
    "label": "DeepSeek",
    "tags": ["chat", "reasoning"],
    "source": {
      "url": "https://api.deepseek.com/v1",
      "accessMode": "remote_api",
      "token": "sk-xxxxxxxxxxxxxxxxxxxxx"
    }
  }'
```

### Example 2: Add a Local Model with Download

```bash
curl -X POST http://localhost:8080/api/v1/playground/models \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{
    "source": {
      "url": "Qwen/Qwen2.5-7B-Instruct",
      "accessMode": "local",
      "token": "hf_xxxxxxxxxxxxxxxxxxxxx"
    },
    "downloadTarget": {
      "type": "local",
      "localPath": "/data/models/qwen2.5-7b"
    },
    "resources": {
      "cpu": "16",
      "memory": "64Gi",
      "gpu": "2"
    }
  }'
```

### Example 3: List Models with Filtering

```bash
# List all running inference services
curl -X GET "http://localhost:8080/api/v1/playground/models?inferenceStatus=Running&limit=20&offset=0" \
  -H "Authorization: Bearer <your-token>"

# List all local models
curl -X GET "http://localhost:8080/api/v1/playground/models?accessMode=local" \
  -H "Authorization: Bearer <your-token>"
```

### Example 4: Start Inference Service

```bash
curl -X POST http://localhost:8080/api/v1/playground/models/model-a3k9x/toggle \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{
    "enabled": true
  }'
```

### Example 5: Stop Inference Service

```bash
curl -X POST http://localhost:8080/api/v1/playground/models/model-a3k9x/toggle \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{
    "enabled": false
  }'
```

### Example 6: Delete Model

```bash
curl -X DELETE http://localhost:8080/api/v1/playground/models/model-a3k9x \
  -H "Authorization: Bearer <your-token>"
```

---

## Architecture Notes

### Data Flow

1.  **API Layer** (`apiserver/pkg/handlers/model-handlers`):
    - Handles HTTP requests
    - Creates/deletes Kubernetes custom resources (Model, Inference)
    - Manages Secrets for tokens

2.  **Controller Layer** (`resource-manager/pkg/resource`):
    - `ModelReconciler`: Manages model download lifecycle
    - `InferenceReconciler`: Manages inference service deployment
    - Creates Kubernetes Jobs for model downloads
    - Creates Workloads for inference services

3.  **Database Sync** (`resource-manager/pkg/exporter`):
    - Watches Kubernetes CRD changes
    - Automatically syncs to PostgreSQL database
    - Implements soft delete (sets `is_deleted = true`)

### Security

- Tokens are stored in Kubernetes Secrets, never in CRDs or database
- Secret names follow the pattern: `{model-id}` (same as model ID)
- Secrets are automatically deleted when the model is deleted

### Storage Options

**Local Storage**:
- Uses Kubernetes HostPath volumes
- Model files are stored on the host node's filesystem
- Suitable for single-node or shared filesystem setups

**S3 Storage**:
- Downloads to temporary container storage
- Uploads to S3 bucket using AWS CLI
- Suitable for distributed deployments and cloud environments

---

## Related APIs

- [Inference API](./inference.md) - Low-level inference service management
- [Playground Sessions API](./playground-sessions.md) - Chat session management
- [Playground Chat API](./playground-chat.md) - Real-time chat interface

