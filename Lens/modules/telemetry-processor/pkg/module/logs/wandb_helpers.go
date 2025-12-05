package logs

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/pods"
	"github.com/sirupsen/logrus"
)

// resolveWorkloadUID resolves WorkloadUID from PodName
// Prioritizes the provided workloadUID (backward compatibility), otherwise queries by podName
func resolveWorkloadUID(workloadUID, podName string) (string, error) {
	// 1. If workloadUID is provided, use it directly (backward compatibility)
	if workloadUID != "" {
		logrus.Debugf("Using provided workload_uid: %s", workloadUID)
		return workloadUID, nil
	}

	// 2. PodName must be provided
	if podName == "" {
		return "", fmt.Errorf("either workload_uid or pod_name is required")
	}

	// 3. Query workloads by podName
	workloads := pods.GetWorkloadsByPodName(podName)
	if len(workloads) == 0 {
		return "", fmt.Errorf("no workload found for pod: %s", podName)
	}

	// 4. If there's only one workload, return it directly
	if len(workloads) == 1 {
		resolvedUID := workloads[0][1]
		logrus.Infof("Resolved pod %s -> workload %s (%s)", podName, workloads[0][0], resolvedUID)
		return resolvedUID, nil
	}

	// 5. If there are multiple workloads, select the top-level workload without parent
	logrus.Infof("Pod %s belongs to %d workloads, filtering for top-level workload without parent",
		podName, len(workloads))

	ctx := context.Background()
	topLevelWorkloads := [][]string{}

	for _, workload := range workloads {
		workloadUID := workload[1]
		workloadName := workload[0]

		// Query workload details
		gpuWorkload, err := database.GetFacade().GetWorkload().GetGpuWorkloadByUid(ctx, workloadUID)
		if err != nil {
			logrus.Warnf("Failed to get workload details for %s: %v", workloadUID, err)
			continue
		}

		if gpuWorkload == nil {
			logrus.Warnf("Workload %s not found in database", workloadUID)
			continue
		}

		// Check if it's a top-level workload (without parent)
		if gpuWorkload.ParentUID == "" {
			topLevelWorkloads = append(topLevelWorkloads, []string{workloadName, workloadUID})
			logrus.Debugf("Found top-level workload: %s (%s)", workloadName, workloadUID)
		} else {
			logrus.Debugf("Skipping child workload: %s (%s), parent: %s",
				workloadName, workloadUID, gpuWorkload.ParentUID)
		}
	}

	// 6. Return based on filtering results
	if len(topLevelWorkloads) == 0 {
		// All workloads have parents, fallback to using the first one
		logrus.Warnf("No top-level workload found for pod %s, using first workload: %s",
			podName, workloads[0][1])
		return workloads[0][1], nil
	}

	if len(topLevelWorkloads) > 1 {
		// Multiple top-level workloads found, use the first one and warn
		logrus.Warnf("Pod %s has %d top-level workloads, using first one: %s (%s)",
			podName, len(topLevelWorkloads), topLevelWorkloads[0][0], topLevelWorkloads[0][1])
	}

	resolvedUID := topLevelWorkloads[0][1]
	logrus.Infof("Resolved pod %s -> top-level workload %s (%s)",
		podName, topLevelWorkloads[0][0], resolvedUID)

	return resolvedUID, nil
}
