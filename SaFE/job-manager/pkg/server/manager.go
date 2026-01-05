/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
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
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/dispatcher"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/failover"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/scheduler"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/netutil"
)

// JobManager represents the main job manager that coordinates various controllers
type JobManager struct {
	Context     context.Context
	CtrlManager manager.Manager
}

// NewJobManager creates and returns a new JobManager instance.
func NewJobManager(scheme *runtime.Scheme) (*JobManager, error) {
	jm := &JobManager{
		Context: ctrlruntime.SetupSignalHandler(),
	}
	var err error
	jm.CtrlManager, err = newCtrlManager(scheme)
	if err != nil {
		return nil, err
	}
	if err = jm.SetupControllers(); err != nil {
		return nil, err
	}
	return jm, nil
}

// newCtrlManager creates and configures a new controller manager.
func newCtrlManager(scheme *runtime.Scheme) (ctrlruntime.Manager, error) {
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
		LeaderElectionID:           "primus-job-manager",
		HealthProbeBindAddress:     healthProbeAddress,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		Controller: config.Controller{
			SkipNameValidation: ptr.To(true),
		},
	}
	cfg, err := commonclient.GetRestConfigInCluster()
	if err != nil {
		return nil, err
	}
	mgr, err := manager.New(cfg, opts)
	if err != nil {
		return nil, err
	}
	if commonconfig.IsHealthCheckEnabled() {
		if err = mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
			return nil, fmt.Errorf("failed to set up health check: %v", err)
		}
		if err = mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
			return nil, fmt.Errorf("failed to set up ready check: %v", err)
		}
	}
	return mgr, nil
}

// SetupControllers initializes and registers all required controllers with the manager.
// Registers syncer, scheduler, dispatcher, failover, and TTL controllers.
func (jm *JobManager) SetupControllers() error {
	if err := syncer.SetupSyncerController(jm.Context, jm.CtrlManager); err != nil {
		return fmt.Errorf("syncer controller: %v", err)
	}
	if err := scheduler.SetupSchedulerController(jm.Context, jm.CtrlManager); err != nil {
		return fmt.Errorf("scheduler controller: %v", err)
	}
	if err := dispatcher.SetupDispatcherController(jm.CtrlManager); err != nil {
		return fmt.Errorf("dispatcher controller: %v", err)
	}
	if err := failover.SetupFailoverController(jm.CtrlManager); err != nil {
		return fmt.Errorf("failover controller: %v", err)
	}
	if err := scheduler.SetupWorkloadTTLController(jm.CtrlManager); err != nil {
		return fmt.Errorf("workload ttl controller: %v", err)
	}
	return nil
}

// Start begins the controller manager and waits for cache synchronization.
// Runs the manager in a goroutine and checks for cache sync.
func (jm *JobManager) Start() error {
	go func() {
		if err := jm.CtrlManager.Start(jm.Context); err != nil {
			klog.ErrorS(err, "failed to start controller manager")
			os.Exit(-1)
		}
	}()
	if !jm.CtrlManager.GetCache().WaitForCacheSync(jm.Context) {
		klog.Error("failed to WaitForCacheSync")
		os.Exit(-1)
	}
	return nil
}

// Wait blocks until the job manager context is cancelled.
// This method should be called to keep the application running.
func (jm *JobManager) Wait() {
	<-jm.Context.Done()
}
