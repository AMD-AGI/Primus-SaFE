/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package server

import (
	"context"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/netutil"
)

// ControllerManager wraps the controller-runtime manager and provides lifecycle management
type ControllerManager struct {
	ctrlManager manager.Manager
	ctx         context.Context
}

// NewControllerManager: creates and configures a new ControllerManager instance
// It sets up leader election, health checks
func NewControllerManager(scheme *runtime.Scheme) (*ControllerManager, error) {
	cm := &ControllerManager{
		ctx: ctrlruntime.SetupSignalHandler(),
	}
	healthProbeAddress := ""
	if commonconfig.IsHealthCheckEnabled() {
		localIp, err := netutil.GetLocalIp()
		if err != nil {
			return nil, err
		}
		if commonconfig.GetHealthCheckPort() <= 0 {
			return nil, fmt.Errorf("the healthcheck port is not defined")
		}
		healthProbeAddress = fmt.Sprintf("%s:%d", localIp, commonconfig.GetHealthCheckPort())
	}

	opts := manager.Options{
		Scheme:                     scheme,
		LeaderElection:             commonconfig.IsLeaderElectionEnable(),
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		LeaderElectionNamespace:    common.PrimusSafeNamespace,
		LeaderElectionID:           "primus-resource-manager",
		HealthProbeBindAddress:     healthProbeAddress,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		Controller: config.Controller{
			MaxConcurrentReconciles: 10,
			SkipNameValidation:      ptr.To(true),
		},
	}

	cfg, err := commonclient.GetRestConfigInCluster()
	if err != nil {
		return nil, err
	}
	cm.ctrlManager, err = manager.New(cfg, opts)
	if err != nil {
		return nil, err
	}
	if commonconfig.IsHealthCheckEnabled() {
		if err = cm.ctrlManager.AddHealthzCheck("healthz", healthz.Ping); err != nil {
			return nil, fmt.Errorf("failed to set up health check: %v", err)
		}
		if err = cm.ctrlManager.AddReadyzCheck("readyz", healthz.Ping); err != nil {
			return nil, fmt.Errorf("failed to set up ready check: %v", err)
		}
	}
	return cm, nil
}

// Start: begins the controller manager operation in a separate goroutine
// It also waits for cache synchronization before returning
func (cm *ControllerManager) Start() error {
	go func() {
		if err := cm.ctrlManager.Start(cm.ctx); err != nil {
			klog.ErrorS(err, "failed to start controller manager")
			os.Exit(-1)
		}
	}()
	if !cm.ctrlManager.GetCache().WaitForCacheSync(cm.ctx) {
		klog.Errorf("failed to WaitForCacheSync")
		os.Exit(-1)
	}
	klog.Info("Controller manager started successfully")
	return nil
}

// Wait: blocks until the controller manager context is cancelled (shutdown signal)
func (cm *ControllerManager) Wait() {
	<-cm.ctx.Done()
}
