/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func SetupControllers(mgr manager.Manager) error {
	if err := SetupNodeController(mgr); err != nil {
		return fmt.Errorf("fail to set up node controller: %v", err)
	}
	return nil
}
