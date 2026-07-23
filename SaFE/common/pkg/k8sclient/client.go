/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package k8sclient

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	// dialTimeout bounds a single TCP dial so a dead endpoint fails fast and
	// failover can move on to the next one.
	dialTimeout = 10 * time.Second
	// dialKeepAlive makes the kernel probe idle connections, so a half-open
	// connection (peer rebooted without sending FIN/RST) is detected and torn
	// down, which unblocks the informer's watch and triggers a reconnect.
	dialKeepAlive = 30 * time.Second
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

// NewClientSet creates and returns a new ClientSet instance for a single endpoint.
func NewClientSet(endpoint, certData, keyData, caData string,
	insecure bool) (kubernetes.Interface, *rest.Config, error) {
	return NewClientSetWithEndpoints([]string{endpoint}, certData, keyData, caData, insecure)
}

// NewClientSetWithEndpoints creates a ClientSet whose transport dials the given
// HA apiserver endpoints with keepalive and failover: the first endpoint is the
// authority (Host), and dials fail over to the others when it is unreachable.
func NewClientSetWithEndpoints(endpoints []string, certData, keyData, caData string,
	insecure bool) (kubernetes.Interface, *rest.Config, error) {
	restConfig, err := createRestConfig(endpoints, certData, keyData, caData, insecure)
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
// endpoints[0] is the authority; extra endpoints are failover dial targets.
func createRestConfig(endpoints []string, certData, keyData, caData string, insecure bool) (*rest.Config, error) {
	cert := stringutil.Base64Decode(certData)
	key := stringutil.Base64Decode(keyData)
	if len(endpoints) == 0 || endpoints[0] == "" || cert == "" || key == "" {
		return nil, fmt.Errorf("invalid input")
	}
	cfg := &rest.Config{
		Host: endpoints[0],
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: insecure,
			KeyData:  []byte(key),
			CertData: []byte(cert),
		},
		QPS:   common.DefaultQPS,
		Burst: common.DefaultBurst,
		Dial:  clusterDialer(endpoints),
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

// clusterDialer returns a DialContext that (1) enables TCP keepalive so a
// half-open connection to a rebooted apiserver is detected and the watch
// reconnects, and (2) fails over across all known HA endpoints so a reconnect
// lands on a healthy apiserver instead of pinning to a single (possibly dead)
// node. Safe with InsecureSkipVerify clients: dialing any HA member under the
// same authority needs no per-target SNI/SAN.
func clusterDialer(endpoints []string) func(context.Context, string, string) (net.Conn, error) {
	base := &net.Dialer{Timeout: dialTimeout, KeepAlive: dialKeepAlive}
	hostPorts := make([]string, 0, len(endpoints))
	for _, ep := range endpoints {
		if hp := hostPortFromEndpoint(ep); hp != "" {
			hostPorts = append(hostPorts, hp)
		}
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if len(hostPorts) <= 1 {
			return base.DialContext(ctx, network, addr)
		}
		// Rotate the start so a persistently dead endpoint is not always tried
		// first (which would waste a dial timeout on every reconnect).
		start := int(time.Now().UnixNano()%int64(len(hostPorts)) + int64(len(hostPorts))) % len(hostPorts)
		var lastErr error
		for i := 0; i < len(hostPorts); i++ {
			cand := hostPorts[(start+i)%len(hostPorts)]
			conn, err := base.DialContext(ctx, network, cand)
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
}

// hostPortFromEndpoint strips the URL scheme/trailing slash from an endpoint,
// leaving a host:port suitable for dialing.
func hostPortFromEndpoint(endpoint string) string {
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	return strings.TrimSuffix(endpoint, "/")
}
