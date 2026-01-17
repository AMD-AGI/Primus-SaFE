# Dataset API

## Overview

Dataset API provides endpoints for managing datasets used in AI/ML training and inference tasks. When creating a dataset, files are uploaded to S3 and automatically downloaded to workspace PFS storage.

## Database Migration

The dataset table is automatically created by the Helm chart. See `charts/primus-safe/templates/database/sql_config.yaml`.

## Dataset Types

| Type | Label | Description |
|------|-------|-------------|
| `sft` | SFT (Supervised Fine-Tuning) | Dataset for supervised fine-tuning |
| `dpo` | DPO (Direct Preference Optimization) | Dataset for preference optimization |
| `pretrain` | Pretrain | Dataset for large-scale pretraining |
| `rlhf` | RLHF | Dataset for reinforcement learning from human feedback |
| `inference` | Inference | Dataset for batch inference |
| `evaluation` | Evaluation | Dataset for model evaluation and benchmarking |
| `other` | Other | Other types of datasets |

## Dataset Status

| Status | Description |
|--------|-------------|
| `Ready` | Dataset is ready for use |

## API Endpoints

### List Dataset Types

List all available dataset types with descriptions.

**Request**

```
GET /api/v1/datasets/types
```

**Response**

```json
{
  "types": [
    {"value": "sft", "label": "SFT (Supervised Fine-Tuning)", "description": "Dataset for supervised fine-tuning"},
    {"value": "dpo", "label": "DPO (Direct Preference Optimization)", "description": "Dataset for preference optimization"},
    {"value": "pretrain", "label": "Pretrain", "description": "Dataset for large-scale pretraining"},
    {"value": "rlhf", "label": "RLHF", "description": "Dataset for reinforcement learning from human feedback"},
    {"value": "inference", "label": "Inference", "description": "Dataset for batch inference"},
    {"value": "evaluation", "label": "Evaluation", "description": "Dataset for model evaluation and benchmarking"},
    {"value": "other", "label": "Other", "description": "Other types of datasets"}
  ]
}
```

### Get Dataset Template

Get a template example for a specific dataset type.

**Request**

```
GET /api/v1/datasets/templates/{type}
```

**Path Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| type | string | Dataset type (sft/dpo/pretrain/rlhf/inference/evaluation/other) |

**Response Examples**

#### SFT Template

```json
{
  "type": "sft",
  "description": "SFT dataset for supervised fine-tuning with instruction-response pairs",
  "format": "jsonl",
  "schema": {
    "instruction": "string (required) - User input or question",
    "input": "string (optional) - Additional context",
    "output": "string (required) - Expected model response"
  },
  "example": "{\"instruction\": \"Translate to English\", \"input\": \"Bonjour\", \"output\": \"Hello\"}\n{\"instruction\": \"Write a poem about spring\", \"output\": \"Spring arrives with gentle rain...\"}"
}
```

#### DPO Template

```json
{
  "type": "dpo",
  "description": "DPO dataset for direct preference optimization with chosen and rejected responses",
  "format": "jsonl",
  "schema": {
    "prompt": "string (required) - The input prompt",
    "chosen": "string (required) - The preferred response",
    "rejected": "string (required) - The less preferred response"
  },
  "example": "{\"prompt\": \"Explain quantum computing\", \"chosen\": \"Quantum computing uses quantum bits...\", \"rejected\": \"It's just faster computers...\"}"
}
```

#### Pretrain Template

```json
{
  "type": "pretrain",
  "description": "Pretrain dataset for large-scale language model pretraining",
  "format": "jsonl or txt",
  "schema": {
    "text": "string (required) - Raw text content for pretraining"
  },
  "example": "{\"text\": \"The quick brown fox jumps over the lazy dog. This is a sample document for pretraining.\"}"
}
```

#### RLHF Template

```json
{
  "type": "rlhf",
  "description": "RLHF dataset for reinforcement learning from human feedback",
  "format": "jsonl",
  "schema": {
    "prompt": "string (required) - The input prompt",
    "response": "string (required) - Model response",
    "reward": "float (required) - Human feedback score",
    "preference": "string (optional) - Preference ranking"
  },
  "example": "{\"prompt\": \"Write a helpful response\", \"response\": \"Here's how I can help...\", \"reward\": 0.85}"
}
```

#### Inference Template

```json
{
  "type": "inference",
  "description": "Inference dataset for batch inference tasks",
  "format": "jsonl",
  "schema": {
    "id": "string (optional) - Unique identifier for the request",
    "prompt": "string (required) - Input prompt for inference"
  },
  "example": "{\"id\": \"req_001\", \"prompt\": \"Summarize this article: ...\"}\n{\"id\": \"req_002\", \"prompt\": \"Translate: Hello world\"}"
}
```

#### Evaluation Template

```json
{
  "type": "evaluation",
  "description": "Evaluation dataset for model benchmarking and testing",
  "format": "jsonl",
  "schema": {
    "question": "string (required) - Test question or prompt",
    "answer": "string (required) - Expected answer",
    "category": "string (optional) - Category or topic",
    "difficulty": "string (optional) - Difficulty level",
    "reference": "string (optional) - Reference or source",
    "answer_choices": "array (optional) - Multiple choice options"
  },
  "example": "{\"question\": \"What is 2+2?\", \"answer\": \"4\", \"category\": \"math\", \"difficulty\": \"easy\"}"
}
```

#### Other Template

```json
{
  "type": "other",
  "description": "Custom dataset format for other use cases",
  "format": "jsonl, csv, or txt",
  "schema": {
    "data": "any - Custom data structure based on your needs"
  },
  "example": "{\"data\": \"Your custom data format here\"}"
}
```

### Create Dataset

Create a new dataset with file upload. Files are uploaded to S3 and automatically downloaded to workspace PFS.

**Request**

```
POST /api/v1/datasets
Content-Type: multipart/form-data
```

**Form Fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| displayName | string | Yes | Display name of the dataset |
| description | string | No | Description of the dataset |
| datasetType | string | Yes | Type of the dataset (sft/dpo/pretrain/rlhf/inference/evaluation/other) |
| workspace | string | No | Target workspace ID. If empty, downloads to all workspaces (deduplicated by path) |
| files | file[] | Yes | Files to upload |

**Response**

```json
{
  "datasetId": "dataset-a1b2c3d4",
  "displayName": "My Training Data",
  "description": "Training data for LLM fine-tuning",
  "datasetType": "sft",
  "status": "Ready",
  "s3Path": "datasets/dataset-a1b2c3d4/",
  "totalSize": 52428800,
  "totalSizeStr": "50.00 MB",
  "fileCount": 2,
  "userId": "user-123",
  "userName": "John Doe",
  "creationTime": "2025-01-15T10:30:00Z",
  "updateTime": "2025-01-15T10:30:00Z",
  "downloadJobs": [
    {
      "jobId": "dataset-dl-dataset-a1b2c3d4-xyz123",
      "workspace": "workspace-1",
      "destPath": "/apps/datasets/My Training Data"
    },
    {
      "jobId": "dataset-dl-dataset-a1b2c3d4-abc456",
      "workspace": "workspace-2",
      "destPath": "/data/datasets/My Training Data"
    }
  ]
}
```

**Download Behavior**

| workspace parameter | Behavior |
|---------------------|----------|
| Specified | Downloads only to the specified workspace's PFS |
| Empty/Not provided | Downloads to all workspaces (paths deduplicated, PFS prioritized) |

The download jobs are created as OpsJobs (type: `download`) that run asynchronously. You can check job status via the OpsJob API.

**OpsJob Parameters**

| Parameter | Value | Description |
|-----------|-------|-------------|
| endpoint | `{s3Endpoint}/{s3Bucket}/datasets/{datasetId}/` | S3 full HTTP URL |
| dest.path | `datasets/{displayName}` | Relative path, controller appends workspace nfsPath |
| secret | `primus-safe-s3` | S3 access credentials |
| workspace | workspace ID | Target workspace |

### List Datasets

List datasets with filtering and pagination.

**Request**

```
GET /api/v1/datasets
```

**Query Parameters**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| datasetType | string | - | Filter by dataset type |
| search | string | - | Search by display name (fuzzy match) |
| pageNum | int | 1 | Page number |
| pageSize | int | 20 | Page size |
| orderBy | string | creation_time | Order by field |
| order | string | desc | Order direction (asc/desc) |

**Response**

```json
{
  "total": 100,
  "pageNum": 1,
  "pageSize": 20,
  "items": [
    {
      "datasetId": "dataset-a1b2c3d4",
      "displayName": "My Training Data",
      "description": "Training data for LLM fine-tuning",
      "datasetType": "sft",
      "status": "Ready",
      "s3Path": "datasets/dataset-a1b2c3d4/",
      "totalSize": 52428800,
      "totalSizeStr": "50.00 MB",
      "fileCount": 2,
      "userId": "user-123",
      "userName": "John Doe",
      "creationTime": "2025-01-15T10:30:00Z",
      "updateTime": "2025-01-15T10:30:00Z"
    }
  ]
}
```

### Get Dataset

Get dataset details by ID.

**Request**

```
GET /api/v1/datasets/{id}
```

**Path Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| id | string | Dataset ID |

**Response**

```json
{
  "datasetId": "dataset-a1b2c3d4",
  "displayName": "My Training Data",
  "description": "Training data for LLM fine-tuning",
  "datasetType": "sft",
  "status": "Ready",
  "s3Path": "datasets/dataset-a1b2c3d4/",
  "totalSize": 52428800,
  "totalSizeStr": "50.00 MB",
  "fileCount": 2,
  "userId": "user-123",
  "userName": "John Doe",
  "creationTime": "2025-01-15T10:30:00Z",
  "updateTime": "2025-01-15T10:30:00Z"
}
```

### Delete Dataset

Delete a dataset by ID. This will also attempt to delete files from S3 (soft delete in database).

**Request**

```
DELETE /api/v1/datasets/{id}
```

**Path Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| id | string | Dataset ID |

**Response**

```json
{
  "message": "dataset deleted successfully",
  "datasetId": "dataset-a1b2c3d4"
}
```

### List Dataset Files

List files in a dataset from S3.

**Request**

```
GET /api/v1/datasets/{id}/files
```

**Path Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| id | string | Dataset ID |

**Response**

```json
{
  "datasetId": "dataset-a1b2c3d4",
  "files": [
    {
      "fileName": "train.jsonl",
      "filePath": "train.jsonl",
      "fileSize": 0,
      "sizeStr": "N/A"
    },
    {
      "fileName": "val.jsonl",
      "filePath": "val.jsonl",
      "fileSize": 0,
      "sizeStr": "N/A"
    }
  ],
  "total": 2
}
```

## Local File Access

After a dataset is downloaded to workspace PFS, you can access the files:

1. **In Workloads**: Mount the workspace volume and access files at `{mount_path}/datasets/{dataset_displayName}/`
2. **Via SSH**: Connect to workspace nodes and navigate to the PFS directory
3. **Example path**: `/apps/datasets/My Training Data/train.jsonl`

## Architecture

### Download Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Dataset Creation Flow                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. POST /api/v1/datasets                                                   │
│       │                                                                     │
│       ▼                                                                     │
│  2. Upload files to S3                                                      │
│       │  - Store at: s3://{bucket}/datasets/{dataset-id}/                   │
│       ▼                                                                     │
│  3. Save metadata to PostgreSQL                                             │
│       │                                                                     │
│       ▼                                                                     │
│  4. Create OpsJob(s) for download                                           │
│       │  - Type: download                                                   │
│       │  - One job per unique workspace path                                │
│       ▼                                                                     │
│  5. OpsJob Controller creates Workload                                      │
│       │  - Image: s3-downloader                                             │
│       │  - Env: INPUT_URL, DEST_PATH, SECRET_PATH                           │
│       ▼                                                                     │
│  6. Download to workspace PFS                                               │
│       │  - Final path: {workspace-pfs}/datasets/{displayName}/              │
│       ▼                                                                     │
│  7. Dataset Ready for use                                                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Storage Paths

| Storage | Path Pattern | Example |
|---------|--------------|---------|
| S3 | `datasets/{datasetId}/` | `datasets/dataset-a1b2c3d4/` |
| Local PFS | `{workspace_mount}/datasets/{displayName}/` | `/apps/datasets/My Training Data/` |
