// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dag

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// terminalIntentStates are intent_state values that signal classification is done.
var terminalIntentStates = map[string]bool{
	"completed":      true,
	"low_confidence": true,
	"failed":         true,
}

// WaitClassificationExecutor is the T6 executor that polls the workload
// detection record until intent classification reaches a terminal state.
type WaitClassificationExecutor struct {
	detectionFacade database.WorkloadDetectionFacadeInterface
}

// NewWaitClassificationExecutor creates a T6 executor.
func NewWaitClassificationExecutor() *WaitClassificationExecutor {
	return &WaitClassificationExecutor{
		detectionFacade: database.NewWorkloadDetectionFacade(),
	}
}

// Execute checks the intent_state field on the workload detection record.
// Returns an error (triggering retry) if classification is still in progress.
func (e *WaitClassificationExecutor) Execute(ctx context.Context, master *MasterTask, sub *SubTask) error {
	detection, err := e.detectionFacade.GetDetection(ctx, master.WorkloadUID)
	if err != nil {
		return fmt.Errorf("failed to get detection for %s: %w", master.WorkloadUID, err)
	}
	if detection == nil {
		return fmt.Errorf("detection record not found for workload %s", master.WorkloadUID)
	}

	intentState := ""
	if detection.IntentState != nil {
		intentState = *detection.IntentState
	}

	if terminalIntentStates[intentState] {
		sub.Result = map[string]interface{}{
			"intent_state": intentState,
			"category":     ptrStr(detection.Category),
		}
		log.Infof("WaitClassificationExecutor: classification done for workload %s (state=%s)",
			master.WorkloadUID, intentState)
		return nil
	}

	return fmt.Errorf("intent classification still in progress for workload %s (state=%s), will retry",
		master.WorkloadUID, intentState)
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
