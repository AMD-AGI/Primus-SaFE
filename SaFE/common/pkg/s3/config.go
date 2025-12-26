/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package s3

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"k8s.io/utils/pointer"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

type Config struct {
	aws.Config
	Bucket *string
}

type S3Location struct {
	Bucket   string
	Endpoint string
	Key      string
}

// NewConfig creates and returns a new S3 configuration object using system-wide S3 settings.
// This function retrieves S3 configuration parameters from the system config
func NewConfig() (*Config, error) {
	if !commonconfig.IsS3Enable() {
		return nil, fmt.Errorf("s3 is disabled")
	}
	if commonconfig.GetS3Bucket() == "" {
		return nil, fmt.Errorf("the s3 bucket is empty")
	}
	return newConfigFromCredentials(commonconfig.GetS3AccessKey(), commonconfig.GetS3SecretKey(),
		commonconfig.GetS3Endpoint(), commonconfig.GetS3Bucket())
}

// NewConfigFromCredentials creates and returns a new S3 configuration object using the provided credentials and url
func NewConfigFromCredentials(ak, sk, s3Url string) (*Config, *S3Location, error) {
	loc, err := parseS3PathStyleURL(s3Url)
	if err != nil {
		return nil, nil, err
	}
	conf, err := newConfigFromCredentials(ak, sk, loc.Endpoint, loc.Bucket)
	if err != nil {
		return nil, nil, err
	}
	return conf, loc, nil
}

// newConfigFromCredentials creates and returns a new S3 configuration object using the provided credentials
func newConfigFromCredentials(ak, sk, endpoint, bucket string) (*Config, error) {
	if ak == "" {
		return nil, fmt.Errorf("the s3 AccessKey is empty")
	}
	if sk == "" {
		return nil, fmt.Errorf("the s3 SecretKey is empty")
	}
	if endpoint == "" {
		return nil, fmt.Errorf("the s3 endpoint is empty")
	}
	if bucket == "" {
		return nil, fmt.Errorf("the s3 bucket is empty")
	}

	credProvider := aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		return aws.Credentials{
			AccessKeyID:     ak,
			SecretAccessKey: sk,
			Source:          "StaticCredentials",
		}, nil
	})

	// Create HTTP client that skips TLS verification
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(""),
		config.WithCredentialsProvider(credProvider),
		config.WithHTTPClient(httpClient),
		config.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL: endpoint,
				}, nil
			}),
		),
	)
	if err != nil {
		return nil, err
	}
	return &Config{
		Config: cfg,
		Bucket: pointer.String(bucket),
	}, nil
}

// parseS3PathStyleURL parses a URL in the format https://<endpoint>/<bucket>/<key>.
func parseS3PathStyleURL(s3url string) (*S3Location, error) {
	if s3url == "" {
		return nil, fmt.Errorf("URL is empty")
	}
	u, err := url.Parse(s3url)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, fmt.Errorf("only http/https schemes are supported")
	}

	host := u.Host
	path := u.Path
	if host == "" {
		return nil, fmt.Errorf("missing host in URL")
	}
	if path == "" || path == "/" {
		return nil, fmt.Errorf("missing bucket and key in path")
	}

	cleanPath := strings.TrimPrefix(path, "/")
	parts := strings.SplitN(cleanPath, "/", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid path: expected '/<bucket>/<key>', got '%s'", path)
	}
	bucket := parts[0]
	key := parts[1]
	if bucket == "" || key == "" {
		return nil, fmt.Errorf("bucket or key is empty in path '%s'", path)
	}
	return &S3Location{
		Bucket:   bucket,
		Endpoint: u.Scheme + "://" + host,
		Key:      key,
	}, nil
}
