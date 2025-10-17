/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package k8sclient

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

// NewClientSetInCluster creates a new Kubernetes clientset and REST config for in-cluster usage
// Returns:
//
//	kubernetes.Interface: The Kubernetes client interface
//	*rest.Config: The REST configuration
//	error: Any error encountered during client creation
func NewClientSetInCluster() (kubernetes.Interface, *rest.Config, error) {
	restConfig, err := GetRestConfigInCluster()
	if err != nil {
		return nil, nil, err
	}
	cli, err := NewClientSetWithRestConfig(restConfig)
	return cli, restConfig, err
}

// NewClientSet creates a new Kubernetes clientset and REST config with provided TLS configuration
// Parameters:
//
//	endpoint: The Kubernetes API server endpoint
//	certData: Base64 encoded client certificate data
//	keyData: Base64 encoded client key data
//	caData: Base64 encoded CA certificate data
//	insecure: Whether to skip TLS verification
//
// Returns:
//
//	kubernetes.Interface: The Kubernetes client interface
//	*rest.Config: The REST configuration
//	error: Any error encountered during client creation
func NewClientSet(endpoint, certData, keyData, caData string,
	insecure bool) (kubernetes.Interface, *rest.Config, error) {
	restConfig, err := createRestConfig(endpoint, certData, keyData, caData, insecure)
	if err != nil {
		return nil, nil, err
	}
	cli, err := NewClientSetWithRestConfig(restConfig)
	return cli, restConfig, err
}

// NewClientSetWithRestConfig creates a new Kubernetes clientset using the provided REST config
// Parameters:
//
//	cfg: The REST configuration for Kubernetes client
//
// Returns:
//
//	kubernetes.Interface: The Kubernetes client interface
//	error: Any error encountered during client creation
func NewClientSetWithRestConfig(cfg *rest.Config) (kubernetes.Interface, error) {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// GetRestConfigInCluster retrieves the REST configuration for in-cluster Kubernetes access
// Returns:
//
//	*rest.Config: The REST configuration with default QPS and Burst settings
//	error: Any error encountered during config retrieval
func GetRestConfigInCluster() (*rest.Config, error) {
	restCfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	restCfg.QPS = common.DefaultQPS
	restCfg.Burst = common.DefaultBurst
	return restCfg, nil
}

// createRestConfig creates a REST configuration with provided TLS parameters
// Parameters:
//
//	endpoint: The Kubernetes API server endpoint
//	certData: Base64 encoded client certificate data
//	keyData: Base64 encoded client key data
//	caData: Base64 encoded CA certificate data
//	insecure: Whether to skip TLS verification
//
// Returns:
//
//	*rest.Config: The configured REST configuration
//	error: Any error encountered during config creation
func createRestConfig(endpoint, certData, keyData, caData string, insecure bool) (*rest.Config, error) {
	cert := stringutil.Base64Decode(certData)
	key := stringutil.Base64Decode(keyData)
	if endpoint == "" || cert == "" || key == "" {
		return nil, fmt.Errorf("invalid input")
	}
	cfg := &rest.Config{
		Host: endpoint,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: insecure,
			KeyData:  []byte(key),
			CertData: []byte(cert),
		},
		QPS:   common.DefaultQPS,
		Burst: common.DefaultBurst,
	}
	if !insecure {
		ca := stringutil.Base64Decode(caData)
		if ca == "" {
			return nil, fmt.Errorf("invalid input")
		}
		cfg.TLSClientConfig.CAData = []byte(ca)
	}
	return cfg, nil
}
