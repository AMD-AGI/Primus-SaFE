/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func SetupControllers(ctx context.Context, mgr manager.Manager) error {
	if err := SetupClusterController(mgr); err != nil {
		return fmt.Errorf("failed to set up cluster controller: %+v", err)
	}
	if err := SetupNodeController(mgr); err != nil {
		return fmt.Errorf("failed to set up node controller: %v", err)
	}
	if err := SetupNodeK8sController(ctx, mgr); err != nil {
		return fmt.Errorf("failed to set up node controller: %v", err)
	}
	if err := SetupWorkspaceController(mgr); err != nil {
		return fmt.Errorf("failed to set up workspace controller: %v", err)
	}
	if err := SetupStorageClusterController(mgr); err != nil {
		return fmt.Errorf("failed to set up storage cluster controller: %+v", err)
	}
	return nil
}
