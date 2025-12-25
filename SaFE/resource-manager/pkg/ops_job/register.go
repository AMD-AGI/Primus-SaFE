/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// SetupOpsJobs initializes and registers all OpsJob controllers with the controller manager.
func SetupOpsJobs(ctx context.Context, mgr manager.Manager) error {
	if err := SetupJobTTLController(mgr); err != nil {
		return fmt.Errorf("job-ttl controller: %v", err)
	}
	if err := SetupAddonJobController(mgr); err != nil {
		return fmt.Errorf("addon-job controller: %v", err)
	}
	if err := SetupDumpLogJobController(ctx, mgr); err != nil {
		return fmt.Errorf("dumplog-job controller: %v", err)
	}
	if err := SetupPreflightJobController(mgr); err != nil {
		return fmt.Errorf("preflight-job controller: %v", err)
	}
	if err := SetupRebootJobController(mgr); err != nil {
		return fmt.Errorf("reboot-job controller: %v", err)
	}
	if err := SetupExportImageJobController(ctx, mgr); err != nil {
		return fmt.Errorf("exportimage-job controller: %v", err)
	}
	if err := SetupPrewarmJobController(ctx, mgr); err != nil {
		return fmt.Errorf("prewarm-job controller: %v", err)
	}
	if err := SetupDownloadJobController(mgr); err != nil {
		return fmt.Errorf("download-job controller: %v", err)
	}
	if err := SetupCDJobController(mgr); err != nil {
		return fmt.Errorf("cd-job controller: %v", err)
	}
	return nil
}
