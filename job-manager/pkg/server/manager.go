/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/dispatcher"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/failover"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/scheduler"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/netutil"
)

type JobManager struct {
	Context     context.Context
	CtrlManager manager.Manager
}

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
		LeaderElectionNamespace:    commonconfig.GetLeaderElectionLock(),
		LeaderElectionID:           "primus-job-manager",
		HealthProbeBindAddress:     healthProbeAddress,
		Metrics: metricsserver.Options{
			BindAddress: "0",
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

func (jm *JobManager) SetupControllers() error {
	if err := syncer.SetupSyncerController(jm.Context, jm.CtrlManager); err != nil {
		return fmt.Errorf("failed to set up workload k8smgr: %v", err)
	}
	if err := scheduler.SetupSchedulerController(jm.Context, jm.CtrlManager); err != nil {
		return fmt.Errorf("failed to set up workload scheduler: %v", err)
	}
	if err := dispatcher.SetupDispatcherController(jm.CtrlManager); err != nil {
		return fmt.Errorf("failed to set up workload dispatcher: %v", err)
	}
	if commonconfig.IsWorkloadFailoverEnable() {
		if err := failover.SetupFailoverController(jm.CtrlManager); err != nil {
			return fmt.Errorf("failed to set up failover controller: %v", err)
		}
	}
	if err := scheduler.SetupWorkloadTTLController(jm.CtrlManager); err != nil {
		return fmt.Errorf("failed to set up workload ttol controller: %v", err)
	}
	return nil
}

func (jm *JobManager) Start() error {
	go func() {
		if err := jm.CtrlManager.Start(jm.Context); err != nil {
			klog.ErrorS(err, "failed to start controller manager")
			os.Exit(-1)
		}
	}()
	if !jm.CtrlManager.GetCache().WaitForCacheSync(jm.Context) {
		klog.Errorf("failed to WaitForCacheSync")
		os.Exit(-1)
	}
	return nil
}

func (jm *JobManager) Wait() {
	<-jm.Context.Done()
}
