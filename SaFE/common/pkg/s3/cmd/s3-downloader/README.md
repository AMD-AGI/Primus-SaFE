# S3 Downloader

A lightweight Docker container for downloading files from S3-compatible object storage.

## Docker Image

- **Repository**: `primussafe/s3-downloader`
- **Tags**: Timestamped (e.g., `202501241430`) and `latest`

## Quick Start

### Pull from Docker Hub

```bash
docker pull primussafe/s3-downloader:latest
```

### Run the container

```bash
docker run --rm \
  -e SECRET_PATH=/run/secrets \
  -e INPUT_URL=s3://s3.example.com/bucket/path/to/file.tar.gz \
  -e DEST_PATH=/data/file.tar.gz \
  -v /path/to/secrets:/run/secrets:ro \
  -v /path/to/output:/data \
  primussafe/s3-downloader:latest
```

## Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `SECRET_PATH` | Directory containing S3 credentials | `/run/secrets` |
| `INPUT_URL` | S3 URL to download | `s3://endpoint.com/bucket/key` |
| `DEST_PATH` | Local destination path | `/data/file.tar.gz` |

## Secret Files

The `SECRET_PATH` directory must contain:

- `access_key` - S3 access key ID
- `secret_key` - S3 secret access key

Example:
```bash
mkdir -p /tmp/s3-secrets
echo "AKIAIOSFODNN7EXAMPLE" > /tmp/s3-secrets/access_key
echo "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" > /tmp/s3-secrets/secret_key
chmod 600 /tmp/s3-secrets/*
```

## Features

- ✅ Automatic handling of small and large files
- ✅ Concurrent multipart download for files > 1GB
- ✅ Progress reporting with download duration
- ✅ Automatic cleanup on failure
- ✅ Runs as non-root user for security

## Building and Publishing

### Prerequisites

```bash
docker login
```

### Build and Push

```bash
cd common/pkg/s3/cmd/s3-downloader
make docker-push
```

This will:
1. Build the Docker image
2. Tag with timestamp (e.g., `202501241430`) and `latest`
3. Push both tags to `primussafe/s3-downloader`

### Build Only (Local)

```bash
make docker-build
```

## Kubernetes Example

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: s3-download-job
spec:
  template:
    spec:
      containers:
      - name: downloader
        image: primussafe/s3-downloader:latest
        env:
        - name: SECRET_PATH
          value: /run/secrets
        - name: INPUT_URL
          value: s3://s3.example.com/bucket/model.tar.gz
        - name: DEST_PATH
          value: /data/model.tar.gz
        volumeMounts:
        - name: s3-credentials
          mountPath: /run/secrets
          readOnly: true
        - name: data
          mountPath: /data
      restartPolicy: OnFailure
      volumes:
      - name: s3-credentials
        secret:
          secretName: s3-credentials
      - name: data
        persistentVolumeClaim:
          claimName: model-data
```

## Performance

- Small files (< 1GB): Single-request download
- Large files (≥ 1GB): Concurrent multipart download
  - Part size: 100MB
  - Concurrency: 5 parallel connections

## License

Copyright (C) 2025 Advanced Micro Devices, Inc. All rights reserved.
