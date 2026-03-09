// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dag

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/common"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// WaitPodReadyExecutor is the T3 executor that checks whether at least one pod
// for the workload is in Ready state. If not ready, it returns an error which
// causes the scheduler to retry on the next tick.
type WaitPodReadyExecutor struct {
	podProber *common.PodProber
}

// NewWaitPodReadyExecutor creates a T3 executor.
func NewWaitPodReadyExecutor(podProber *common.PodProber) *WaitPodReadyExecutor {
	return &WaitPodReadyExecutor{
		podProber: podProber,
	}
}

// Execute checks pod readiness. Returns an error (triggering retry) if no pod
// is ready yet.
func (e *WaitPodReadyExecutor) Execute(ctx context.Context, master *MasterTask, sub *SubTask) error {
	pods, err := e.podProber.ListPodsByWorkload(ctx, master.WorkloadUID)
	if err != nil {
		return fmt.Errorf("failed to list pods for workload %s: %w", master.WorkloadUID, err)
	}

	if len(pods) == 0 {
		return fmt.Errorf("no pods found for workload %s, will retry", master.WorkloadUID)
	}

	for _, pod := range pods {
		if e.podProber.IsPodReady(ctx, pod) {
			sub.Result = map[string]interface{}{
				"ready_pod_uid": pod.PodUID,
				"pod_name":      pod.PodName,
			}
			log.Debugf("WaitPodReadyExecutor: pod %s is ready for workload %s", pod.PodName, master.WorkloadUID)
			return nil
		}
	}

	return fmt.Errorf("no ready pods for workload %s (%d pods found), will retry", master.WorkloadUID, len(pods))
}
