/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package server

import (
	"context"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"
	ctrlruntime "sigs.k8s.io/controller-runtime"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/netutil"
)

func TestNewControllerManagerProbe(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(ctrlruntime.SetupSignalHandler, func() context.Context { return context.Background() })
	patches.ApplyFunc(commonclient.GetRestConfigInCluster, func() (*rest.Config, error) {
		return &rest.Config{Host: "http://127.0.0.1:60999"}, nil
	})
	patches.ApplyFunc(commonconfig.IsHealthCheckEnabled, func() bool { return false })
	patches.ApplyFunc(commonconfig.IsMetricsEnabled, func() bool { return false })
	patches.ApplyFunc(commonconfig.IsLeaderElectionEnable, func() bool { return false })

	cm, err := NewControllerManager(scheme)
	assert.NoError(t, err)
	assert.NotNil(t, cm)
}

func TestNewControllerManagerHealthAndMetrics(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(ctrlruntime.SetupSignalHandler, func() context.Context { return context.Background() })
	patches.ApplyFunc(commonclient.GetRestConfigInCluster, func() (*rest.Config, error) {
		return &rest.Config{Host: "http://127.0.0.1:60999"}, nil
	})
	patches.ApplyFunc(netutil.GetLocalIp, func() (string, error) { return "127.0.0.1", nil })
	patches.ApplyFunc(commonconfig.IsHealthCheckEnabled, func() bool { return true })
	patches.ApplyFunc(commonconfig.GetHealthCheckPort, func() int { return 18081 })
	patches.ApplyFunc(commonconfig.IsMetricsEnabled, func() bool { return true })
	patches.ApplyFunc(commonconfig.GetMetricsPort, func() int { return 18082 })
	patches.ApplyFunc(commonconfig.IsLeaderElectionEnable, func() bool { return false })

	cm, err := NewControllerManager(scheme)
	assert.NoError(t, err)
	assert.NotNil(t, cm)
}

func TestNotificationRunnerStartDisabled(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(commonconfig.IsNotificationEnable, func() bool { return false })

	runner := &NotificationRunner{}
	assert.NoError(t, runner.Start(context.Background()))
}
