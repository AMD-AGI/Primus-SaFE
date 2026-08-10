/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package k8sclient

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	// DefaultAPIServerProbeTimeout bounds a single apiserver reachability probe.
	DefaultAPIServerProbeTimeout = 10 * time.Second
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

// NewClientSetWithProbe builds a client and verifies apiserver reachability via ServerVersion.
// It tries primary first, then each fallback endpoint until one succeeds. The caller's deadline is
// honoured, so probing a run of unreachable endpoints cannot outlast it.
func NewClientSetWithProbe(ctx context.Context, primary string, fallbackEndpoints []string,
	certData, keyData, caData string, insecure bool,
) (kubernetes.Interface, *rest.Config, string, error) {
	candidates := uniqueEndpoints(append([]string{primary}, fallbackEndpoints...))
	if len(candidates) == 0 {
		return nil, nil, "", fmt.Errorf("no apiserver endpoint candidates")
	}

	var lastErr error
	for _, endpoint := range candidates {
		if err := ctx.Err(); err != nil {
			lastErr = err
			break
		}
		clientSet, restCfg, err := NewClientSet(endpoint, certData, keyData, caData, insecure)
		if err != nil {
			lastErr = err
			continue
		}
		if err = probeRESTConfig(restCfg, probeTimeout(ctx)); err != nil {
			lastErr = err
			continue
		}
		return clientSet, restCfg, endpoint, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("all apiserver probes failed")
	}
	return nil, nil, "", fmt.Errorf("no reachable apiserver endpoint: %w", lastErr)
}

// ProbeRESTConfig verifies apiserver reachability with a bounded ServerVersion call.
func ProbeRESTConfig(restCfg *rest.Config) error {
	return probeRESTConfig(restCfg, DefaultAPIServerProbeTimeout)
}

// ProbeRESTConfigWithContext verifies apiserver reachability without outliving the caller.
func ProbeRESTConfigWithContext(ctx context.Context, restCfg *rest.Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return probeRESTConfig(restCfg, probeTimeout(ctx))
}

// probeRESTConfig runs a ServerVersion call bounded by timeout. The discovery client takes no
// context, so a deadline can only be applied through the REST client timeout.
func probeRESTConfig(restCfg *rest.Config, timeout time.Duration) error {
	if restCfg == nil {
		return fmt.Errorf("rest config is nil")
	}
	cfg := rest.CopyConfig(restCfg)
	cfg.Timeout = timeout
	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	_, err = clientSet.Discovery().ServerVersion()
	return err
}

// probeTimeout caps a probe at the caller's remaining budget. A non-positive rest.Config timeout
// means "no timeout", so an expired deadline is floored instead of removing the bound entirely.
func probeTimeout(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return DefaultAPIServerProbeTimeout
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return time.Millisecond
	}
	if remaining < DefaultAPIServerProbeTimeout {
		return remaining
	}
	return DefaultAPIServerProbeTimeout
}

// ProbeAPIServer verifies apiserver reachability using the REST config behind the client.
func ProbeAPIServer(_ context.Context, clientSet kubernetes.Interface, restCfg *rest.Config) error {
	if restCfg != nil {
		return ProbeRESTConfig(restCfg)
	}
	if clientSet == nil {
		return fmt.Errorf("client set is nil")
	}
	_, err := clientSet.Discovery().ServerVersion()
	return err
}

// NormalizeEndpointHost ensures the apiserver host includes an HTTP scheme.
func NormalizeEndpointHost(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return endpoint
	}
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}
	return "https://" + endpoint
}

func uniqueEndpoints(endpoints []string) []string {
	seen := make(map[string]struct{}, len(endpoints))
	out := make([]string, 0, len(endpoints))
	for _, ep := range endpoints {
		ep = NormalizeEndpointHost(ep)
		if ep == "" {
			continue
		}
		if _, ok := seen[ep]; ok {
			continue
		}
		seen[ep] = struct{}{}
		out = append(out, ep)
	}
	return out
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
	endpoint = NormalizeEndpointHost(endpoint)
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
