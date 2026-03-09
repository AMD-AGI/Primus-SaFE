// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package config

import (
	"encoding/base64"
	"fmt"

	"gopkg.in/yaml.v3"
	"k8s.io/client-go/rest"
)

// kubeConfigV1 is a minimal kubeconfig structure for serialization.
type kubeConfigV1 struct {
	APIVersion     string                 `yaml:"apiVersion"`
	Kind           string                 `yaml:"kind"`
	Clusters       []kubeConfigNamedCluster `yaml:"clusters"`
	Users          []kubeConfigNamedUser   `yaml:"users"`
	Contexts       []kubeConfigNamedContext `yaml:"contexts"`
	CurrentContext string                 `yaml:"current-context"`
}

type kubeConfigNamedCluster struct {
	Name    string `yaml:"name"`
	Cluster struct {
		Server                   string `yaml:"server"`
		CertificateAuthorityData string `yaml:"certificate-authority-data"`
	} `yaml:"cluster"`
}

type kubeConfigNamedUser struct {
	Name string `yaml:"name"`
	User struct {
		Token string `yaml:"token"`
	} `yaml:"user"`
}

type kubeConfigNamedContext struct {
	Name    string `yaml:"name"`
	Context struct {
		Cluster string `yaml:"cluster"`
		User    string `yaml:"user"`
	} `yaml:"context"`
}

// BuildInClusterKubeconfig builds kubeconfig bytes from the in-cluster service account
// so that executor and Helm can use the same cluster. Call only when running inside a pod.
func BuildInClusterKubeconfig() ([]byte, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("not in cluster or in-cluster config failed: %w", err)
	}

	caData := ""
	if len(cfg.CAData) > 0 {
		caData = base64.StdEncoding.EncodeToString(cfg.CAData)
	}

	kc := kubeConfigV1{
		APIVersion: "v1",
		Kind:       "Config",
		Clusters: []kubeConfigNamedCluster{{
			Name: "in-cluster",
			Cluster: struct {
				Server                   string `yaml:"server"`
				CertificateAuthorityData string `yaml:"certificate-authority-data"`
			}{
				Server:                   cfg.Host,
				CertificateAuthorityData: caData,
			},
		}},
		Users: []kubeConfigNamedUser{{
			Name: "in-cluster",
			User: struct {
				Token string `yaml:"token"`
			}{Token: cfg.BearerToken},
		}},
		Contexts: []kubeConfigNamedContext{{
			Name: "in-cluster",
			Context: struct {
				Cluster string `yaml:"cluster"`
				User    string `yaml:"user"`
			}{
				Cluster: "in-cluster",
				User:    "in-cluster",
			},
		}},
		CurrentContext: "in-cluster",
	}

	out, err := yaml.Marshal(kc)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal kubeconfig: %w", err)
	}
	return out, nil
}
