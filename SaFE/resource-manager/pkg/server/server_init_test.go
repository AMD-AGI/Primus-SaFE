/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package server

import (
	"context"
	"reflect"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonklog "github.com/AMD-AIG-AIMA/SAFE/common/pkg/klog"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/options"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/exporter"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/informer"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/ops_job"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/resource"
)

func serverDummyManager(t *testing.T) manager.Manager {
	t.Helper()
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = v1.AddToScheme(s)
	mgr, err := ctrlruntime.NewManager(&rest.Config{Host: "http://127.0.0.1:60999"}, ctrlruntime.Options{
		Scheme:                 s,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		Controller:             ctrlconfig.Controller{SkipNameValidation: ptr.To(true)},
	})
	assert.NoError(t, err)
	return mgr
}

func TestNewServerWithStubs(t *testing.T) {
	dummyMgr := serverDummyManager(t)
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	patches.ApplyMethod(reflect.TypeOf(&options.Options{}), "InitFlags",
		func(_ *options.Options) error { return nil })
	patches.ApplyFunc(commonklog.Init, func(_ string, _ int) error { return nil })
	patches.ApplyFunc(commonconfig.LoadConfig, func(_ string) error { return nil })
	patches.ApplyFunc(NewControllerManager, func(_ *runtime.Scheme) (*ControllerManager, error) {
		return &ControllerManager{ctx: context.Background(), ctrlManager: dummyMgr}, nil
	})
	patches.ApplyFunc(resource.SetupControllers, func(_ context.Context, _ manager.Manager) error { return nil })
	patches.ApplyFunc(ops_job.SetupOpsJobs, func(_ context.Context, _ manager.Manager) error { return nil })
	patches.ApplyFunc(exporter.SetupExporters, func(_ context.Context, _ manager.Manager) error { return nil })

	s, err := NewServer()
	assert.NoError(t, err)
	assert.NotNil(t, s)
	assert.True(t, s.isInited)
}

func TestServerStartNotInited(t *testing.T) {
	s := &Server{opts: &options.Options{}, isInited: false}
	// Should return early without panicking because the server is not initialized.
	s.Start()
}

func TestControllerManagerWaitCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cm := &ControllerManager{ctx: ctx}
	cm.Wait()
}

func TestServerInitLogsAndConfig(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(commonklog.Init, func(_ string, _ int) error { return nil })
	patches.ApplyFunc(commonconfig.LoadConfig, func(_ string) error { return nil })

	s := &Server{opts: &options.Options{Config: "cfg.yaml"}}
	assert.NoError(t, s.initLogs())
	assert.NoError(t, s.initConfig())
}

func TestNotificationRunnerStartEnabled(t *testing.T) {
	dummyMgr := serverDummyManager(t)
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(commonconfig.IsNotificationEnable, func() bool { return true })
	patches.ApplyFunc(commonconfig.GetNotificationConfig, func() string { return "cfg" })
	patches.ApplyFunc(notification.InitNotificationManager, func(_ context.Context, _ string) error { return nil })
	patches.ApplyFunc(informer.InitInformer, func(_ context.Context, _ *rest.Config, _ ctrlclient.Client) error { return nil })

	runner := &NotificationRunner{ctrlManager: &ControllerManager{ctx: context.Background(), ctrlManager: dummyMgr}}
	err := runner.Start(context.Background())
	assert.NoError(t, err)
}
