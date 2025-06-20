/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package job

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func SetupJobs(ctx context.Context, mgr manager.Manager) error {
	if err := SetupJobTTLController(mgr); err != nil {
		return fmt.Errorf("failed to set up job-ttl controller: %+v", err)
	}
	if err := SetupAddonJobController(ctx, mgr); err != nil {
		return fmt.Errorf("failed to set up addon-job controller: %+v", err)
	}
	return nil
}
