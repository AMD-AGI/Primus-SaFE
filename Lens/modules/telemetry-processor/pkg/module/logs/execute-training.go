package logs

import (
	"context"
	"strings"
)

func executeWorkloadLog(ctx context.Context, workloadLog *PodLog) error {
	if workloadLog.Kubernetes == nil {
		return nil
	}
	if strings.Contains(workloadLog.Kubernetes.PodName, "primus-lens-telemetry-processor") {
		return nil
	}
	err := WorkloadLog(ctx,
		workloadLog.Kubernetes.PodId,
		workloadLog.Message,
		workloadLog.Time)
	if err != nil {
		return err
	}
	return nil
}
