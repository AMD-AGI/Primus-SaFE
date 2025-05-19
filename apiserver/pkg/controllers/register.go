/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controllers

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func SetupControllers(mgr manager.Manager) error {
	if err := SetupClusterController(mgr); err != nil {
		return fmt.Errorf("failed to set up cluster controller: %+v", err)
	}
	return nil
}
