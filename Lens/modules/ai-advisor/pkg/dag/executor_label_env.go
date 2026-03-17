// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dag

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// LabelEnvExecutor is the T2 executor that verifies the workload detection
// record exists. Labels and environment variables are already available in the
// spec (collected by T1), so this step is a lightweight validation gate.
type LabelEnvExecutor struct {
	detectionFacade database.WorkloadDetectionFacadeInterface
}

// NewLabelEnvExecutor creates a T2 executor.
func NewLabelEnvExecutor() *LabelEnvExecutor {
	return &LabelEnvExecutor{
		detectionFacade: database.NewWorkloadDetectionFacade(),
	}
}

// Execute verifies the workload detection record exists.
func (e *LabelEnvExecutor) Execute(ctx context.Context, master *MasterTask, sub *SubTask) error {
	detection, err := e.detectionFacade.GetDetection(ctx, master.WorkloadUID)
	if err != nil {
		return fmt.Errorf("failed to get detection for %s: %w", master.WorkloadUID, err)
	}
	if detection == nil {
		return fmt.Errorf("no detection record found for workload %s", master.WorkloadUID)
	}

	sub.Result = map[string]interface{}{
		"detection_status": detection.Status,
		"framework":        detection.Framework,
		"workload_type":    detection.WorkloadType,
	}

	log.Debugf("LabelEnvExecutor: detection verified for workload %s (framework=%s)",
		master.WorkloadUID, detection.Framework)
	return nil
}
