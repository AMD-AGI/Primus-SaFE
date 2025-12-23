/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controllers

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// SetupControllers sets up all controllers with the manager.
func SetupControllers(ctx context.Context, mgr manager.Manager) error {
	if err := SetupClusterController(ctx, mgr); err != nil {
		return fmt.Errorf("cluster controller: %v", err)
	}
	if err := SetupCDOpsJobController(ctx, mgr); err != nil {
		return fmt.Errorf("cd opsjob controller: %v", err)
	}
	return nil
}
