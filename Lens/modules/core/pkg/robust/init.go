// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package robust

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"k8s.io/klog/v2"
)

var defaultClient *Client

// Init initializes the Robust data plane integration based on config.
// Call this during server startup after config is loaded.
func Init(ctx context.Context, cfg *config.Config) {
	if cfg.GetDataPlaneMode() == "local" {
		klog.Info("[robust] data plane mode: local (no Robust integration)")
		return
	}

	baseURL := cfg.DataPlane.Robust.BaseURL
	if baseURL == "" {
		klog.Warning("[robust] data plane mode is not local but no baseUrl configured, falling back to local")
		return
	}

	timeout := cfg.DataPlane.Robust.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	client := NewClient(baseURL, timeout)

	if err := client.HealthCheck(ctx); err != nil {
		klog.Warningf("[robust] health check failed (%s): %v", baseURL, err)
		if !cfg.ShouldFallbackToLocal() {
			klog.Warning("[robust] fallbackToLocal is disabled, proceeding anyway")
		} else {
			klog.Warning("[robust] falling back to local data plane")
			return
		}
	} else {
		klog.Infof("[robust] health check passed: %s", baseURL)
	}

	defaultClient = client

	database.SetRobustFacadeFactory(func(localFacade *database.Facade, clusterName string) database.FacadeInterface {
		url := cfg.GetRobustBaseURL(clusterName)
		if url == "" {
			return localFacade
		}
		c := client
		if url != baseURL {
			c = NewClient(url, timeout)
		}
		return NewRobustFacade(localFacade, c, clusterName)
	})

	klog.Infof("[robust] data plane enabled: mode=%s baseUrl=%s", cfg.GetDataPlaneMode(), baseURL)
}

// GetClient returns the default Robust client (nil if not initialized).
func GetClient() *Client {
	return defaultClient
}
