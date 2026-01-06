/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package informer

import (
	"context"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/informers/externalversions"
)

// InitInformer initializes Kubernetes informers for resource monitoring.
func InitInformer(ctx context.Context, cfg *rest.Config, controllerRuntimeClient client.Client) error {
	versionedClient, err := versioned.NewForConfig(cfg)
	if err != nil {
		return err
	}
	workloadInformer := NewWorkloadInformer(controllerRuntimeClient)
	factory := externalversions.NewSharedInformerFactory(versionedClient, 0)
	err = workloadInformer.Register(factory)
	if err != nil {
		return err
	}
	factory.Start(ctx.Done())
	return nil
}
