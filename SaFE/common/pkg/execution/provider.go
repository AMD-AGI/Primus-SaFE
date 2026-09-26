/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package execution

import (
	"crypto/sha256"
	"errors"
	"sync"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

var (
	sharedMu          sync.Mutex
	sharedClient      *Client
	sharedFingerprint [sha256.Size]byte
)

// Shared returns the process-wide client for the configured capacity controller.
//
// One deployment talks to one controller, which is also the single active mutator of the
// execution ledger, so a single client is the right shape and its connection pool is worth
// keeping. The client is rebuilt when the endpoint or the mTLS material changes, which is
// what makes certificate rotation take effect without a restart.
func Shared() (*Client, error) {
	if !commonconfig.IsExternalExecutionEnable() {
		return nil, errors.New("external execution: feature is not enabled")
	}
	baseURL := commonconfig.GetExternalControllerURL()
	if baseURL == "" {
		return nil, errors.New("external execution: controller url is not configured")
	}
	caCert, clientCert, clientKey := commonconfig.GetExternalControllerTLS()
	if len(caCert) == 0 || len(clientCert) == 0 || len(clientKey) == 0 {
		// Refused rather than downgraded. An unverified connection to the component that
		// grants reservations is indistinguishable from a working one until something has
		// already been placed on capacity nobody authorised.
		return nil, errors.New("external execution: controller mTLS material is not configured")
	}

	fingerprint := sha256.Sum256(append(append(append(
		[]byte(baseURL), caCert...), clientCert...), clientKey...))

	sharedMu.Lock()
	defer sharedMu.Unlock()
	if sharedClient != nil && sharedFingerprint == fingerprint {
		return sharedClient, nil
	}
	client, err := NewClient(Config{
		BaseURL:    baseURL,
		CACert:     caCert,
		ClientCert: clientCert,
		ClientKey:  clientKey,
		Timeout:    commonconfig.GetExternalControllerTimeout(),
	})
	if err != nil {
		return nil, err
	}
	sharedClient = client
	sharedFingerprint = fingerprint
	return sharedClient, nil
}
