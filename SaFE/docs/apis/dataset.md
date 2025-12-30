# Dataset API

## Overview

Dataset API provides endpoints for managing datasets used in AI/ML training and inference tasks.

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

### Create Dataset

Create a new dataset with file upload.

**Request**

```
POST /api/v1/datasets
Content-Type: multipart/form-data

Fields:
  displayName  string   Required. Display name of the dataset
  description  string   Optional. Description of the dataset
  datasetType  string   Required. Type of the dataset (sft/dpo/pretrain/rlhf/inference/evaluation/other)
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
  "updateTime": "2025-01-15T10:30:00Z"
}
```

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

Delete a dataset by ID.

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

List files in a dataset.

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
