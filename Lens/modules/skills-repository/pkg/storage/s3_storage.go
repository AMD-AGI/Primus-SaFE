// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package storage

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Storage implements Storage interface for S3/MinIO
type S3Storage struct {
	client    *s3.Client
	bucket    string
	presigner *s3.PresignClient
	urlExpiry time.Duration
}

// S3Config contains S3 configuration
type S3Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
	URLExpiry       time.Duration
}

// NewS3Storage creates a new S3Storage
func NewS3Storage(cfg S3Config) (*S3Storage, error) {
	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:               cfg.Endpoint,
			HostnameImmutable: true,
		}, nil
	})

	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		)),
		config.WithEndpointResolverWithOptions(resolver),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.UsePathStyle
	})

	// Auto-create bucket if it doesn't exist (private by default)
	if err := ensureBucketExists(context.Background(), client, cfg.Bucket); err != nil {
		// Log warning but don't fail - bucket might already exist or user might not have CreateBucket permission
		// The actual upload will fail if bucket doesn't exist
		_ = err
	}

	urlExpiry := cfg.URLExpiry
	if urlExpiry == 0 {
		urlExpiry = 1 * time.Hour
	}

	return &S3Storage{
		client:    client,
		bucket:    cfg.Bucket,
		presigner: s3.NewPresignClient(client),
		urlExpiry: urlExpiry,
	}, nil
}

// ensureBucketExists creates the bucket if it doesn't exist
func ensureBucketExists(ctx context.Context, client *s3.Client, bucket string) error {
	// Check if bucket exists
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err == nil {
		// Bucket already exists
		return nil
	}

	// Try to create the bucket (private by default, no public access)
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		// Ignore "bucket already exists" errors
		// Different S3 implementations return different error types
		return err
	}

	return nil
}

// Upload uploads a file to S3
func (s *S3Storage) Upload(ctx context.Context, key string, reader io.Reader) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   reader,
	})
	return err
}

// UploadBytes uploads bytes to S3
func (s *S3Storage) UploadBytes(ctx context.Context, key string, data []byte) error {
	return s.Upload(ctx, key, bytes.NewReader(data))
}

// Download downloads a file from S3
func (s *S3Storage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return result.Body, nil
}

// DownloadBytes downloads a file and returns its content as bytes
func (s *S3Storage) DownloadBytes(ctx context.Context, key string) ([]byte, error) {
	reader, err := s.Download(ctx, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// Delete deletes a file from S3
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

// Exists checks if a file exists in S3
func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return false, nil
	}
	return true, nil
}

// GetURL returns a presigned URL for the file
func (s *S3Storage) GetURL(ctx context.Context, key string) (string, error) {
	presignResult, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(s.urlExpiry))
	if err != nil {
		return "", err
	}
	return presignResult.URL, nil
}

// ListObjects lists all objects with the given prefix
func (s *S3Storage) ListObjects(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var objects []ObjectInfo

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			objects = append(objects, ObjectInfo{
				Key:  *obj.Key,
				Size: *obj.Size,
			})
		}
	}

	return objects, nil
}

// Copy copies a file from srcKey to dstKey
func (s *S3Storage) Copy(ctx context.Context, srcKey, dstKey string) error {
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		CopySource: aws.String(s.bucket + "/" + srcKey),
		Key:        aws.String(dstKey),
	})
	return err
}

// CopyPrefix copies all files under srcPrefix to dstPrefix
func (s *S3Storage) CopyPrefix(ctx context.Context, srcPrefix, dstPrefix string) error {
	objects, err := s.ListObjects(ctx, srcPrefix)
	if err != nil {
		return err
	}

	for _, obj := range objects {
		// Calculate new key by replacing prefix
		newKey := dstPrefix + obj.Key[len(srcPrefix):]
		if err := s.Copy(ctx, obj.Key, newKey); err != nil {
			return err
		}
	}

	return nil
}
