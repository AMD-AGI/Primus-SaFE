/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package s3

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"k8s.io/utils/pointer"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

type Config struct {
	aws.Config
	Bucket *string
}

// TLSOptions says how an S3 client verifies its endpoint's certificate. The zero value verifies it against the
// system's certificate authorities, as the AWS SDK does.
type TLSOptions struct {
	// CABundle holds PEM certificates of authorities trusted in addition to the system's, for an endpoint whose
	// certificate a private authority signed.
	CABundle string
	// InsecureSkipVerify turns the verification off, for an endpoint whose certificate cannot be verified at all.
	InsecureSkipVerify bool
}

// PlatformTLSOptions returns the TLS options of the platform's S3 endpoint, from the optional ca_bundle and
// insecure_skip_verify items of its secret.
func PlatformTLSOptions() TLSOptions {
	return TLSOptions{CABundle: commonconfig.GetS3CABundle(), InsecureSkipVerify: commonconfig.IsS3InsecureSkipVerify()}
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
		commonconfig.GetS3Endpoint(), commonconfig.GetS3Bucket(), PlatformTLSOptions())
}

// NewConfigFromCredentials creates and returns a new S3 configuration object using the provided credentials and
// url; tlsOpts says how the endpoint's certificate is verified.
func NewConfigFromCredentials(ak, sk, s3Url string, tlsOpts TLSOptions) (*Config, *S3Location, error) {
	loc, err := parseS3PathStyleURL(s3Url)
	if err != nil {
		return nil, nil, err
	}
	conf, err := newConfigFromCredentials(ak, sk, loc.Endpoint, loc.Bucket, tlsOpts)
	if err != nil {
		return nil, nil, err
	}
	return conf, loc, nil
}

// newConfigFromCredentials creates and returns a new S3 configuration object using the provided credentials
func newConfigFromCredentials(ak, sk, endpoint, bucket string, tlsOpts TLSOptions) (*Config, error) {
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

	options := []func(*config.LoadOptions) error{
		config.WithRegion(""),
		config.WithCredentialsProvider(credProvider),
		config.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL: endpoint,
				}, nil
			}),
		),
	}
	// Without TLS options the SDK's own HTTP client stays in place.
	if tlsOpts != (TLSOptions{}) {
		httpClient, err := newHTTPClient(tlsOpts)
		if err != nil {
			return nil, err
		}
		options = append(options, config.WithHTTPClient(httpClient))
	}
	cfg, err := config.LoadDefaultConfig(context.TODO(), options...)
	if err != nil {
		return nil, err
	}
	return &Config{
		Config: cfg,
		Bucket: pointer.String(bucket),
	}, nil
}

// newHTTPClient returns an HTTP client that verifies the endpoint's certificate as opts say: against the system's
// authorities plus those of the CA bundle, or not at all.
func newHTTPClient(opts TLSOptions) (*awshttp.BuildableClient, error) {
	var roots *x509.CertPool
	if opts.CABundle != "" {
		var err error
		if roots, err = x509.SystemCertPool(); err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM([]byte(opts.CABundle)) {
			return nil, fmt.Errorf("the s3 ca_bundle holds no PEM certificate")
		}
	}
	return awshttp.NewBuildableClient().WithTransportOptions(func(tr *http.Transport) {
		setTLS(tr, roots, opts.InsecureSkipVerify)
	}), nil
}

// setTLS makes a transport trust roots, the system's authorities when nil, and skip the verification when asked.
func setTLS(tr *http.Transport, roots *x509.CertPool, insecureSkipVerify bool) {
	if tr.TLSClientConfig == nil {
		tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	tr.TLSClientConfig.RootCAs = roots
	tr.TLSClientConfig.InsecureSkipVerify = insecureSkipVerify //nolint:gosec // only on an explicit opt-out
}

// parseS3PathStyleURL parses a URL in the format https://<endpoint>/<bucket>/<key> or https://<endpoint>/<bucket>/
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
	// Use EscapedPath to preserve URL encoding
	path := u.EscapedPath()
	if host == "" {
		return nil, fmt.Errorf("missing host in URL")
	}
	if path == "" || path == "/" {
		return nil, fmt.Errorf("missing bucket and key in path")
	}

	cleanPath := strings.TrimPrefix(path, "/")
	parts := strings.SplitN(cleanPath, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return nil, fmt.Errorf("missing bucket in path '%s'", path)
	}
	bucket := parts[0]
	// Key can be empty for root bucket access or directory downloads.
	key := ""
	if len(parts) > 1 {
		key = parts[1]
	}
	return &S3Location{
		Bucket:   bucket,
		Endpoint: u.Scheme + "://" + host,
		Key:      key,
	}, nil
}
