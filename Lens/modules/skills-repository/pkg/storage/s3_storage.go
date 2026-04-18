// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package storage

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Storage implements Storage interface for S3/MinIO
type S3Storage struct {
	client          *s3.Client
	bucket          string
	endpoint        string
	publicURL       string // Public URL for file access (optional, may include bucket path)
	publicURLPrefix string // Path prefix extracted from publicURL (e.g., "/s3")
	presigner       *s3.PresignClient
	publicPresigner *s3.PresignClient // Presigner configured with public URL for generating upload links
	urlExpiry       time.Duration
}

// S3Config contains S3 configuration
type S3Config struct {
	Endpoint        string
	PublicURL       string // Public URL for file access (optional)
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

	// Create a separate presigner for public URLs if configured
	var publicPresigner *s3.PresignClient
	var publicURLPrefix string

	if cfg.PublicURL != "" {
		// Parse the public URL to extract the base endpoint and the prefix
		// e.g. https://oci-slc.primus-safe.amd.com/s3/tools
		u, err := url.Parse(cfg.PublicURL)
		if err == nil {
			// Base endpoint for signing MUST NOT include the prefix (e.g., /s3)
			// otherwise the signature will be calculated for /s3/tools/... instead of /tools/...
			publicEndpoint := u.Scheme + "://" + u.Host

			// Extract the prefix (e.g., /s3)
			path := u.Path
			bucketSuffix := "/" + cfg.Bucket
			if strings.HasSuffix(path, bucketSuffix) {
				publicURLPrefix = strings.TrimSuffix(path, bucketSuffix)
			} else if path == cfg.Bucket {
				publicURLPrefix = ""
			} else {
				publicURLPrefix = path
			}

			publicResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:               publicEndpoint,
					HostnameImmutable: true,
				}, nil
			})

			publicAwsCfg, _ := config.LoadDefaultConfig(context.Background(),
				config.WithRegion(cfg.Region),
				config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
					cfg.AccessKeyID,
					cfg.SecretAccessKey,
					"",
				)),
				config.WithEndpointResolverWithOptions(publicResolver),
			)
			publicClient := s3.NewFromConfig(publicAwsCfg, func(o *s3.Options) {
				o.UsePathStyle = cfg.UsePathStyle
			})
			publicPresigner = s3.NewPresignClient(publicClient)
		}
	} else {
		publicPresigner = s3.NewPresignClient(client)
	}

	return &S3Storage{
		client:          client,
		bucket:          cfg.Bucket,
		endpoint:        cfg.Endpoint,
		publicURL:       cfg.PublicURL,
		publicURLPrefix: publicURLPrefix,
		presigner:       s3.NewPresignClient(client),
		publicPresigner: publicPresigner,
		urlExpiry:       urlExpiry,
	}, nil
}

// ensureBucketExists creates the bucket if it doesn't exist and sets it to public read
func ensureBucketExists(ctx context.Context, client *s3.Client, bucket string) error {
	// Check if bucket exists
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err == nil {
		// Bucket already exists, ensure public read policy is set
		return setBucketPublicReadPolicy(ctx, client, bucket)
	}

	// Try to create the bucket
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		// Ignore "bucket already exists" errors
		// Different S3 implementations return different error types
		return nil
	}

	// Set bucket policy to allow public read
	return setBucketPublicReadPolicy(ctx, client, bucket)
}

// setBucketPublicReadPolicy sets the bucket policy to allow public read access
func setBucketPublicReadPolicy(ctx context.Context, client *s3.Client, bucket string) error {
	policy := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": "*",
				"Action": "s3:GetObject",
				"Resource": "arn:aws:s3:::` + bucket + `/*"
			}
		]
	}`

	_, _ = client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: aws.String(bucket),
		Policy: aws.String(policy),
	})

	// Ignore errors - some S3 implementations may not support bucket policies
	// The bucket will still work, just won't be publicly readable
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

// GetURL returns a public URL for the file (no signature, permanent access)
func (s *S3Storage) GetURL(ctx context.Context, key string) (string, error) {
	// Return direct URL without presigning (bucket is public read)
	var url string
	if s.publicURL != "" {
		// Use public URL if configured (already includes bucket path)
		// e.g., http://test.primus-safe.amd.com/minio/tools + /icons/user/file.png
		url = s.publicURL + "/" + key
	} else {
		// Fall back to endpoint + bucket + key
		// e.g., http://minio:9000 + /tools + /icons/user/file.png
		url = s.endpoint + "/" + s.bucket + "/" + key
	}
	return url, nil
}

// GeneratePresignedUploadURL generates a presigned URL for uploading a file directly to S3
func (s *S3Storage) GeneratePresignedUploadURL(ctx context.Context, key string, contentType string, expire time.Duration) (string, error) {
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}

	req, err := s.publicPresigner.PresignPutObject(ctx, input, func(opts *s3.PresignOptions) {
		opts.Expires = expire
	})
	if err != nil {
		return "", err
	}

	finalURL := req.URL
	if s.publicURLPrefix != "" {
		u, err := url.Parse(finalURL)
		if err == nil {
			u.Path = s.publicURLPrefix + u.Path
			finalURL = u.String()
		}
	}

	return finalURL, nil
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
