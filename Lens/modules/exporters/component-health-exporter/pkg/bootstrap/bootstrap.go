// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package bootstrap

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	"github.com/AMD-AGI/Primus-SaFE/Lens/component-health-exporter/pkg/collector"
	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var schemes = &runtime.SchemeBuilder{
	appsv1.AddToScheme,
	corev1.AddToScheme,
}

// init registers the K8s scheme so that when InitClusterManager creates the
// controller-runtime client (before preInit runs), it already knows Deployment,
// DaemonSet, StatefulSet, and Pod types.
func init() {
	if err := controller.RegisterScheme(schemes); err != nil {
		panic(err)
	}
}

// Init runs after the cluster manager is initialized. It registers the
// component-health Prometheus collector so /metrics exposes primus_component_*.
func Init(ctx context.Context, cfg *config.Config) error {
	_ = ctx
	_ = cfg
	if err := prometheus.Register(collector.NewComponentHealthCollector()); err != nil {
		return err
	}
	return nil
}
