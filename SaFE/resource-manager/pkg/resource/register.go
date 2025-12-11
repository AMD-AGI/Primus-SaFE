/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	defaultWorkspaceOption = WorkspaceReconcilerOption{
		processWait: 1 * time.Second,
		nodeWait:    30 * time.Second,
	}
	defaultFaultOption = FaultReconcilerOption{
		maxRetryCount: 30,
		processWait:   1 * time.Second,
	}
)

// SetupControllers initializes and registers all resource controllers with the controller manager.
func SetupControllers(ctx context.Context, mgr manager.Manager) error {
	if err := SetupClusterController(mgr); err != nil {
		return fmt.Errorf("failed to set up cluster controller: %v", err)
	}
	if err := SetupNodeController(mgr); err != nil {
		return fmt.Errorf("failed to set up node controller: %v", err)
	}
	if err := SetupNodeK8sController(ctx, mgr); err != nil {
		return fmt.Errorf("failed to set up k8s-node controller: %v", err)
	}
	if err := SetupWorkspaceController(mgr, &defaultWorkspaceOption); err != nil {
		return fmt.Errorf("failed to set up workspace controller: %v", err)
	}
	if err := SetupFaultController(mgr, &defaultFaultOption); err != nil {
		return fmt.Errorf("failed to set up fault controller: %v", err)
	}
	if err := SetupAddonController(mgr); err != nil {
		return fmt.Errorf("failed to set up addon controller: %+v", err)
	}
	if err := SetupAddonTemplateController(mgr); err != nil {
		return fmt.Errorf("failed to set up addon controller: %+v", err)
	}
	if err := SetupImageImportJobReconciler(mgr); err != nil {
		return fmt.Errorf("failed to set up image import job controller: %v", err)
	}
	if err := SetupSecretController(mgr); err != nil {
		return fmt.Errorf("failed to set up secret controller: %v", err)
	}
	if err := SetupInferenceController(mgr); err != nil {
		return fmt.Errorf("failed to set up inference controller: %v", err)
	}
	if err := SetupModelController(mgr); err != nil {
		return fmt.Errorf("failed to set up model controller: %v", err)
	}
	return nil
}
