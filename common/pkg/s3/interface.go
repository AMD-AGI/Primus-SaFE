/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package s3

import (
	"context"
	"io"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type Interface interface {
	CreateBucket() error
	ListBuckets() (*s3.ListBucketsOutput, error)
	DeleteBucket() error
	IsBucketExisted() (bool, error)

	BeforeMultipart(ctx context.Context, key string, timeout int64) (*s3.S3, string, error)
	MultipartUpload(ctx context.Context, s3Client *s3.S3,
		key, uploadId, value string, partNumber, timeout int64) (*s3.CompletedPart, error)
	AfterMultipart(ctx context.Context, s3Client *s3.S3,
		key, uploadId string, uploadedParts []*s3.CompletedPart, timeout int64) error
	AbortMultipartUpload(s3Client *s3.S3, key, uploadId string) error

	PutObject(ctx context.Context, key, value string, timeout int64) error
	PutObjectWithContext(ctx aws.Context, key string, body io.ReadSeeker) (*s3.PutObjectOutput, error)
	Upload(key string, body io.ReadSeeker) (*s3manager.UploadOutput, error)
	ListObject() (*s3.ListObjectsOutput, error)
	ListObjectWithPrefix(prefix *string, marker *string, delimiter *string, maxKeys *int64) (*s3.ListObjectsOutput, error)
	GetObject(key string) error
	DeleteObject(key string) error
	GetObjectStream(key string) (*s3.GetObjectOutput, error)
	PresignPutObject(ctx context.Context, key string, expire time.Duration) (string, error)
	PresignListObjects(ctx context.Context, prefix string, maxKeys int64, expire time.Duration) (string, error)
	PresignGetObject(ctx context.Context, key string, expire time.Duration) (string, error)
	MoveObject(ctx context.Context, sourceKey string, destinationKey string) error
	CopyObject(ctx context.Context, sourceKey string, destinationKey string) error
	GetS3Config() ClusterConfig
	StreamDownloadFile(key string) (io.ReadCloser, error)
	GetObjectResp(key string) (*s3.GetObjectOutput, error)
	CopyDirectory(ctx context.Context, sourceDir string, destinationDir string) error
}
