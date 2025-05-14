/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package k8sclient

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func NewClientSetInCluster() (kubernetes.Interface, *rest.Config, error) {
	restConfig, err := GetRestConfigInCluster()
	if err != nil {
		return nil, nil, err
	}
	cli, err := NewClientSetWithRestConfig(restConfig)
	return cli, restConfig, err
}

func NewClientSet(endpoint, clientCert, clientKey, clusterCa string,
	insecure bool) (kubernetes.Interface, *rest.Config, error) {
	restConfig, err := GetRestConfig(endpoint, clientCert, clientKey, clusterCa, insecure)
	if err != nil {
		return nil, nil, err
	}
	cli, err := NewClientSetWithRestConfig(restConfig)
	return cli, restConfig, err
}

func NewClientSetWithConfig(kubeconfig string) (kubernetes.Interface, *rest.Config, error) {
	if kubeconfig == "" {
		return nil, nil, fmt.Errorf("the kubconfig is empty")
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, nil, err
	}
	cli, err := NewClientSetWithRestConfig(restConfig)
	return cli, restConfig, err
}

func NewClientSetWithRestConfig(cfg *rest.Config) (kubernetes.Interface, error) {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func GetRestConfig(endpoint, clientCert, clientKey, clusterCa string, insecure bool) (*rest.Config, error) {
	inst := crypto.Instance()
	if inst == nil {
		return nil, fmt.Errorf("failed to new crypto instance")
	}
	cert, err := inst.Decrypt(clientCert)
	if err != nil {
		return nil, err
	}
	key, err := inst.Decrypt(clientKey)
	if err != nil {
		return nil, err
	}
	ca, err := inst.Decrypt(clusterCa)
	if err != nil {
		return nil, err
	}
	restConfig, err := getRestConfig(endpoint, cert, key, ca, insecure)
	if err != nil {
		return nil, err
	}
	return restConfig, nil
}

func GetRestConfigInCluster() (*rest.Config, error) {
	restCfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	restCfg.QPS = common.DefaultQPS
	restCfg.Burst = common.DefaultBurst
	klog.Infof("%+v", restCfg)
	return restCfg, nil
}

func getRestConfig(endpoint, clientCert, clientKey, clusterCa string, insecure bool) (*rest.Config, error) {
	cert := stringutil.Base64Decode(clientCert)
	key := stringutil.Base64Decode(clientKey)
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
		ca := stringutil.Base64Decode(clusterCa)
		if ca == "" {
			return nil, fmt.Errorf("invalid input")
		}
		cfg.TLSClientConfig.CAData = []byte(ca)
	}
	return cfg, nil
}
