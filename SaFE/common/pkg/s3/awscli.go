/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package s3

import (
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

const (
	// AWSCLICABundleEnv is the variable through which a job gets the CA bundle of the platform's S3 endpoint.
	AWSCLICABundleEnv = "S3_CA_BUNDLE"

	// AWSCLITLSSetup is the shell that makes the AWS CLI trust the CA bundle of AWSCLICABundleEnv in addition to
	// the image's authorities. It does nothing while the variable is empty. A job's script runs it before its
	// first aws command.
	AWSCLITLSSetup = `if [ -n "${S3_CA_BUNDLE:-}" ]; then ` +
		`{ cat /etc/ssl/certs/ca-certificates.crt 2>/dev/null || true; printf '%s\n' "$S3_CA_BUNDLE"; } ` +
		`> /tmp/s3-ca-bundle.pem; export AWS_CA_BUNDLE=/tmp/s3-ca-bundle.pem; fi`
)

// AWSCLIEnv returns the variables a job needs so that the AWS CLI verifies the certificate of the platform's S3
// endpoint: the CA bundle, when the S3 secret has one.
func AWSCLIEnv() map[string]string {
	env := map[string]string{}
	if bundle := commonconfig.GetS3CABundle(); bundle != "" {
		env[AWSCLICABundleEnv] = bundle
	}
	return env
}

// AWSCLICommand returns the aws command of a job against the platform's S3 endpoint. It verifies the endpoint's
// certificate unless the S3 secret turns the verification off.
func AWSCLICommand() string {
	if commonconfig.IsS3InsecureSkipVerify() {
		return "aws --no-verify-ssl"
	}
	return "aws"
}
