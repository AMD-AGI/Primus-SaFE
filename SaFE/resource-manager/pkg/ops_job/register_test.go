/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"testing"

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
)

// newDummyManager builds a controller-runtime manager backed by an unreachable
// API server. It is sufficient to exercise controller registration (Setup*)
// without actually connecting to a cluster.
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

func TestSetupOpsJobControllersProbe(t *testing.T) {
	mgr := newDummyManager(t)
	err := SetupOpsJobs(context.Background(), mgr)
	assert.NoError(t, err)
}
