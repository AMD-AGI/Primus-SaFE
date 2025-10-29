/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package s3

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"k8s.io/utils/pointer"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

type Config struct {
	aws.Config
	Bucket *string
}

// GetConfig creates and returns a new S3 configuration object.
// It validates that all required S3 configuration parameters are present in the system config,
// including access key, secret key, endpoint, and bucket name.
//
// Returns:
//   - *Config: A Config struct containing the AWS SDK configuration and S3 bucket name
//   - error: An error if S3 is disabled or any required configuration parameter is missing

func GetConfig() (*Config, error) {
	if !commonconfig.IsS3Enable() {
		return nil, fmt.Errorf("s3 is disabled")
	}
	if commonconfig.GetS3AccessKey() == "" {
		return nil, fmt.Errorf("failed to find AccessKey of s3")
	}
	if commonconfig.GetS3SecretKey() == "" {
		return nil, fmt.Errorf("failed to find SecretKey of s3")
	}
	if commonconfig.GetS3Endpoint() == "" {
		return nil, fmt.Errorf("failed to find endpoint of s3")
	}
	if commonconfig.GetS3Bucket() == "" {
		return nil, fmt.Errorf("failed to find bucket of s3")
	}

	credProvider := aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		return aws.Credentials{
			AccessKeyID:     commonconfig.GetS3AccessKey(),
			SecretAccessKey: commonconfig.GetS3SecretKey(),
			SessionToken:    "",
			Source:          "StaticCredentials",
		}, nil
	})

	region := "us-east-1"
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credProvider),
		config.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:           commonconfig.GetS3Endpoint(),
					SigningRegion: region,
				}, nil
			}),
		),
	)
	if err != nil {
		return nil, err
	}
	return &Config{
		Config: cfg,
		Bucket: pointer.String(commonconfig.GetS3Bucket()),
	}, nil
}

// GetS3Config returns the underlying AWS SDK configuration from the Config struct.
// This provides access to the raw aws.Config object that can be used with AWS SDK S3 operations.
//
// Returns:
//   - aws.Config: The AWS SDK configuration object
func (c *Config) GetS3Config() aws.Config {
	return c.Config
}
