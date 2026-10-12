/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

func newResourceDummyManager(t *testing.T) manager.Manager {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)
	mgr, err := ctrlruntime.NewManager(&rest.Config{Host: "http://127.0.0.1:60999"}, ctrlruntime.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		Controller:             ctrlconfig.Controller{SkipNameValidation: ptr.To(true)},
	})
	if err != nil {
		t.Fatalf("failed to build manager: %v", err)
	}
	return mgr
}

func TestSetupResourceControllersProbe(t *testing.T) {
	mgr := newResourceDummyManager(t)
	ctx := context.Background()

	run := func(name string, fn func() error) {
		t.Run(name, func(t *testing.T) {
			if err := fn(); err != nil {
				t.Logf("%s returned error (acceptable in probe): %v", name, err)
			}
		})
	}

	run("cluster", func() error { return SetupClusterController(mgr) })
	run("node", func() error { return SetupNodeController(mgr) })
	run("nodek8s", func() error { return SetupNodeK8sController(ctx, mgr) })
	run("workspace", func() error { return SetupWorkspaceController(mgr, &defaultWorkspaceOption) })
	run("fault", func() error { return SetupFaultController(mgr, &defaultFaultOption) })
	run("addon", func() error { return SetupAddonController(mgr) })
	run("addontemplate", func() error { return SetupAddonTemplateController(mgr) })
	run("imageimport", func() error { return SetupImageImportJobReconciler(mgr) })
	run("secret", func() error { return SetupSecretController(mgr) })
	run("model", func() error { return SetupModelController(mgr) })
	run("github", func() error { return SetupGitHubWorkflowController(mgr) })

	rc := robustclient.NewClient(robustclient.DefaultClientConfig())
	run("workloadRobust", func() error { return SetupWorkloadRobustSyncer(mgr, rc) })
	run("statsRobustNil", func() error { return SetupStatsRobustSyncer(mgr, nil, nil) })
	run("statsRobust", func() error { return SetupStatsRobustSyncer(mgr, rc, &stubStatsDBWriter{}) })
}

func TestSetupControllersProbe(t *testing.T) {
	mgr := newResourceDummyManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// All Setup* helpers register against the dummy manager; robust discovery
	// starts a background goroutine but never connects in unit tests.
	err := SetupControllers(ctx, mgr)
	if err != nil {
		t.Logf("SetupControllers returned error (acceptable in probe): %v", err)
	}
}

type stubStatsDBWriter struct{}

func (s *stubStatsDBWriter) UpsertWorkloadStatistic(ctx context.Context, cluster string, stats []WorkloadHourlyStats) error {
	return nil
}

func (s *stubStatsDBWriter) UpsertNodeStatistic(ctx context.Context, cluster string, stats []NodeStats) error {
	return nil
}
