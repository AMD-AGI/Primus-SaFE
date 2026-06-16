/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exporter

import (
	"context"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func newDummyManager(t *testing.T) manager.Manager {
	t.Helper()
	scheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(scheme))
	assert.NoError(t, v1.AddToScheme(scheme))
	mgr, err := ctrlruntime.NewManager(&rest.Config{Host: "http://127.0.0.1:60999"}, ctrlruntime.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		Controller:             ctrlconfig.Controller{SkipNameValidation: ptr.To(true)},
	})
	assert.NoError(t, err)
	return mgr
}

func TestSetupExportersDisabled(t *testing.T) {
	patches := gomonkey.NewPatches()
	patches.ApplyFunc(commonconfig.IsDBEnable, func() bool { return false })
	defer patches.Reset()

	assert.NoError(t, SetupExporters(context.Background(), newDummyManager(t)))
}

func TestSetupExportersEnabled(t *testing.T) {
	patches := gomonkey.NewPatches()
	patches.ApplyFunc(commonconfig.IsDBEnable, func() bool { return true })
	patches.ApplyFunc(dbclient.NewClient, func() *dbclient.Client { return &dbclient.Client{} })
	defer patches.Reset()

	assert.NoError(t, SetupExporters(context.Background(), newDummyManager(t)))
}
