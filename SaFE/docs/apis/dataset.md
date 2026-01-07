# Dataset API

## Overview

Dataset API provides endpoints for managing datasets used in AI/ML training and inference tasks. When creating a dataset, files are uploaded to S3 and automatically downloaded to workspace PFS storage.

## Database Migration

The dataset table is automatically created by the Helm chart. See `charts/primus-safe/templates/database/sql_config.yaml`.

## Dataset Types

| Type | Description |
|------|-------------|
| `sft` | SFT (Supervised Fine-Tuning) dataset |
| `dpo` | DPO (Direct Preference Optimization) dataset |
| `pretrain` | Pretrain dataset for large-scale pretraining |
| `rlhf` | RLHF (Reinforcement Learning from Human Feedback) dataset |
| `inference` | Inference dataset for batch inference |
| `evaluation` | Evaluation dataset for model benchmarking |
| `other` | Other types of datasets |

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

**Response** (example for `sft` type)

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

### Create Dataset

Create a new dataset with file upload. Files are uploaded to S3 and automatically downloaded to workspace PFS.

**Request**

```
POST /api/v1/datasets
Content-Type: multipart/form-data

Fields:
  displayName  string   Required. Display name of the dataset
  description  string   Optional. Description of the dataset
  datasetType  string   Required. Type of the dataset (sft/dpo/pretrain/rlhf/inference/evaluation/other)
  workspace    string   Optional. Target workspace ID. If empty, downloads to all workspaces (deduplicated by path)
  files        file[]   Required. Files to upload
```

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
      "jobId": "dataset-dl-a1b2c3d4-xyz",
      "workspace": "workspace-1",
      "destPath": "/apps/datasets/My Training Data"
    },
    {
      "jobId": "dataset-dl-a1b2c3d4-abc",
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

The download jobs are created as OpsJobs that run asynchronously. You can check job status via the OpsJob API.

### List Datasets

List datasets with filtering and pagination.

**Request**

```
GET /api/v1/datasets?datasetType=sft&search=training&pageNum=1&pageSize=20&orderBy=creation_time&order=desc
```

**Query Parameters**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| datasetType | string | - | Filter by dataset type |
| search | string | - | Search by display name |
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

Delete a dataset by ID. This will also attempt to delete files from S3.

**Request**

```
DELETE /api/v1/datasets/{id}
```

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
