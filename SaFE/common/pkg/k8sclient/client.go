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

// NewClientSetInCluster creates and returns a new ClientSetInCluster instance.
func NewClientSetInCluster() (kubernetes.Interface, *rest.Config, error) {
	restConfig, err := GetRestConfigInCluster()
	if err != nil {
		return nil, nil, err
	}
	cli, err := NewClientSetWithRestConfig(restConfig)
	return cli, restConfig, err
}

// NewClientSet creates and returns a new ClientSet instance.
func NewClientSet(endpoint, certData, keyData, caData string,
	insecure bool) (kubernetes.Interface, *rest.Config, error) {
	restConfig, err := createRestConfig(endpoint, certData, keyData, caData, insecure)
	if err != nil {
		return nil, nil, err
	}
	cli, err := NewClientSetWithRestConfig(restConfig)
	return cli, restConfig, err
}

// NewClientSetWithRestConfig creates and returns a new ClientSetWithRestConfig instance.
func NewClientSetWithRestConfig(cfg *rest.Config) (kubernetes.Interface, error) {
	return kubernetes.NewForConfig(cfg)
}

// GetRestConfigInCluster retrieves the REST configuration for in-cluster Kubernetes access.
func GetRestConfigInCluster() (*rest.Config, error) {
	restCfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	restCfg.QPS = common.DefaultQPS
	restCfg.Burst = common.DefaultBurst
	return restCfg, nil
}

// createRestConfig creates a REST configuration with provided TLS parameters.
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
