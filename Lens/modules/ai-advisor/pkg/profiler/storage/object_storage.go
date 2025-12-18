package storage

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ObjectStorageBackend implements StorageBackend using MinIO/S3
type ObjectStorageBackend struct {
	client     *minio.Client
	bucket     string
	urlExpires time.Duration
}

// NewObjectStorageBackend creates a new object storage backend
func NewObjectStorageBackend(config *ObjectStorageConfig) (*ObjectStorageBackend, error) {
	if config == nil {
		return nil, fmt.Errorf("object storage config is nil")
	}

	// Parse URL expiration
	urlExpires := 7 * 24 * time.Hour // Default: 7 days
	if config.URLExpires != "" {
		duration, err := time.ParseDuration(config.URLExpires)
		if err != nil {
			log.Warnf("Invalid url_expires '%s', using default 7 days", config.URLExpires)
		} else {
			urlExpires = duration
		}
	}

	// Create MinIO client
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure: config.UseSSL,
		Region: config.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	backend := &ObjectStorageBackend{
		client:     client,
		bucket:     config.Bucket,
		urlExpires: urlExpires,
	}

	// Ensure bucket exists
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, config.Bucket)
	if err != nil {
		log.Warnf("Failed to check bucket existence: %v", err)
	} else if !exists {
		log.Warnf("Bucket '%s' does not exist, attempting to create it", config.Bucket)
		if err := client.MakeBucket(ctx, config.Bucket, minio.MakeBucketOptions{Region: config.Region}); err != nil {
			log.Warnf("Failed to create bucket: %v", err)
		} else {
			log.Infof("Created bucket '%s'", config.Bucket)
		}
	}

	log.Infof("Initialized object storage backend: endpoint=%s, bucket=%s, url_expires=%v",
		config.Endpoint, config.Bucket, urlExpires)

	return backend, nil
}

// Store stores a file to object storage
func (b *ObjectStorageBackend) Store(ctx context.Context, req *StoreRequest) (*StoreResponse, error) {
	// Generate object key: profiler/{workload}/{date}/{type}/{filename}
	objectKey := b.generateObjectKey(req.WorkloadUID, req.FileType, req.FileName)

	// Calculate MD5
	md5Hash := fmt.Sprintf("%x", md5.Sum(req.Content))

	// Upload to MinIO/S3
	contentReader := bytes.NewReader(req.Content)
	contentSize := int64(len(req.Content))

	uploadInfo, err := b.client.PutObject(ctx, b.bucket, objectKey, contentReader, contentSize, minio.PutObjectOptions{
		ContentType: b.getContentType(req.FileName),
		UserMetadata: map[string]string{
			"workload-uid": req.WorkloadUID,
			"file-type":    req.FileType,
			"compressed":   fmt.Sprintf("%v", req.Compressed),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upload to object storage: %w", err)
	}

	log.Infof("Uploaded file to object storage: key=%s, size=%d bytes, etag=%s",
		objectKey, uploadInfo.Size, uploadInfo.ETag)

	return &StoreResponse{
		FileID:      req.FileID,
		StoragePath: objectKey,
		StorageType: "object_storage",
		Size:        contentSize,
		MD5:         md5Hash,
		Metadata: map[string]interface{}{
			"bucket": b.bucket,
			"etag":   uploadInfo.ETag,
		},
	}, nil
}

// Retrieve retrieves a file from object storage
func (b *ObjectStorageBackend) Retrieve(ctx context.Context, req *RetrieveRequest) (*RetrieveResponse, error) {
	// Get object from MinIO/S3
	object, err := b.client.GetObject(ctx, b.bucket, req.StoragePath, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	defer object.Close()

	// Get object info (for validation)
	_, err = object.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat object: %w", err)
	}

	// Read content
	var content bytes.Buffer
	bytesRead, err := io.Copy(&content, object)
	if err != nil {
		return nil, fmt.Errorf("failed to read object content: %w", err)
	}

	// Apply offset/length if specified
	data := content.Bytes()
	if req.Offset > 0 || req.Length > 0 {
		data = b.extractRange(data, req.Offset, req.Length)
	}

	// Calculate MD5
	md5Hash := fmt.Sprintf("%x", md5.Sum(data))

	log.Debugf("Retrieved file from object storage: key=%s, size=%d bytes",
		req.StoragePath, bytesRead)

	return &RetrieveResponse{
		Content:    data,
		Size:       int64(len(data)),
		Compressed: false, // Object storage doesn't track compression
		MD5:        md5Hash,
	}, nil
}

// Delete deletes a file from object storage
func (b *ObjectStorageBackend) Delete(ctx context.Context, fileID string) error {
	// Note: fileID is the object key in object storage
	err := b.client.RemoveObject(ctx, b.bucket, fileID, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	log.Infof("Deleted file from object storage: key=%s", fileID)
	return nil
}

// GenerateDownloadURL generates a presigned download URL
func (b *ObjectStorageBackend) GenerateDownloadURL(ctx context.Context, fileID string, expires time.Duration) (string, error) {
	// Use configured expiration if not specified
	if expires == 0 {
		expires = b.urlExpires
	}

	// Generate presigned URL
	presignedURL, err := b.client.PresignedGetObject(ctx, b.bucket, fileID, expires, url.Values{})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	log.Debugf("Generated presigned URL: key=%s, expires_in=%v", fileID, expires)
	return presignedURL.String(), nil
}

// GetStorageType returns the storage type identifier
func (b *ObjectStorageBackend) GetStorageType() string {
	return "object_storage"
}

// Exists checks if a file exists in object storage
func (b *ObjectStorageBackend) Exists(ctx context.Context, fileID string) (bool, error) {
	_, err := b.client.StatObject(ctx, b.bucket, fileID, minio.StatObjectOptions{})
	if err != nil {
		// Check if error is "object not found"
		errResponse := minio.ToErrorResponse(err)
		if errResponse.Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("failed to check object existence: %w", err)
	}
	return true, nil
}

// ExistsByWorkloadAndFilename checks if a file with the same name already exists for the workload
// For object storage, we check if the object key pattern exists
func (b *ObjectStorageBackend) ExistsByWorkloadAndFilename(ctx context.Context, workloadUID string, fileName string) (bool, error) {
	// List objects with prefix matching the workload
	prefix := fmt.Sprintf("profiler/%s/", workloadUID)
	objectCh := b.client.ListObjects(ctx, b.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for object := range objectCh {
		if object.Err != nil {
			return false, fmt.Errorf("failed to list objects: %w", object.Err)
		}
		// Check if filename matches
		if len(object.Key) > len(fileName) && object.Key[len(object.Key)-len(fileName):] == fileName {
			return true, nil
		}
	}

	return false, nil
}

// Helper methods

func (b *ObjectStorageBackend) generateObjectKey(workloadUID, fileType, fileName string) string {
	// Format: profiler/{workload}/{date}/{type}/{filename}
	date := time.Now().Format("2006-01-02")
	return fmt.Sprintf("profiler/%s/%s/%s/%s", workloadUID, date, fileType, fileName)
}

func (b *ObjectStorageBackend) getContentType(fileName string) string {
	// Determine content type based on file extension
	if len(fileName) > 3 && fileName[len(fileName)-3:] == ".gz" {
		return "application/gzip"
	}
	if len(fileName) > 5 && fileName[len(fileName)-5:] == ".json" {
		return "application/json"
	}
	if len(fileName) > 7 && fileName[len(fileName)-7:] == ".pickle" {
		return "application/octet-stream"
	}
	return "application/octet-stream"
}

func (b *ObjectStorageBackend) extractRange(data []byte, offset, length int64) []byte {
	size := int64(len(data))

	if offset >= size {
		return []byte{}
	}

	end := size
	if length > 0 && offset+length < size {
		end = offset + length
	}

	return data[offset:end]
}
