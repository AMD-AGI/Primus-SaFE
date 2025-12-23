package common

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/client"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/types"
)

// ProcessTreeOptions defines options for getting process tree
type ProcessTreeOptions struct {
	IncludeEnv       bool
	IncludeCmdline   bool
	IncludeResources bool
}

// DefaultProcessTreeOptions returns default options for process tree
func DefaultProcessTreeOptions() ProcessTreeOptions {
	return ProcessTreeOptions{
		IncludeEnv:       true,
		IncludeCmdline:   true,
		IncludeResources: true,
	}
}

// PodProber provides shared Pod probing capabilities
type PodProber struct {
	collector      *metadata.Collector
	workloadFacade database.WorkloadFacadeInterface
	podFacade      database.PodFacadeInterface
	nodeFacade     database.NodeFacadeInterface
}

// NewPodProber creates a new PodProber instance
func NewPodProber(collector *metadata.Collector) *PodProber {
	return &PodProber{
		collector:      collector,
		workloadFacade: database.GetFacade().GetWorkload(),
		podFacade:      database.GetFacade().GetPod(),
		nodeFacade:     database.GetFacade().GetNode(),
	}
}

// NewPodProberWithFacades creates a PodProber with custom facades (for testing)
func NewPodProberWithFacades(
	collector *metadata.Collector,
	workloadFacade database.WorkloadFacadeInterface,
	podFacade database.PodFacadeInterface,
	nodeFacade database.NodeFacadeInterface,
) *PodProber {
	return &PodProber{
		collector:      collector,
		workloadFacade: workloadFacade,
		podFacade:      podFacade,
		nodeFacade:     nodeFacade,
	}
}

// SelectTargetPod selects a target pod for a workload
// Prioritizes pods with names ending in master-0, otherwise returns the first pod
func (p *PodProber) SelectTargetPod(ctx context.Context, workloadUID string) (*model.GpuPods, error) {
	// Method 1: Find pod through workload_pod_reference table
	podRefs, err := p.workloadFacade.ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUID)
	if err != nil {
		log.Warnf("Failed to query workload_pod_reference for workload %s: %v", workloadUID, err)
	}

	var pods []*model.GpuPods
	if len(podRefs) > 0 {
		podUIDs := make([]string, 0, len(podRefs))
		for _, ref := range podRefs {
			podUIDs = append(podUIDs, ref.PodUID)
		}

		db := database.GetFacade().GetSystemConfig().GetDB()
		err = db.WithContext(ctx).
			Where("uid IN ? AND deleted = ?", podUIDs, false).
			Find(&pods).Error
		if err != nil {
			return nil, fmt.Errorf("failed to query pods by references: %w", err)
		}
	}

	// Method 2: Find pods of child workload
	if len(pods) == 0 {
		childWorkloads, err := p.workloadFacade.ListChildrenWorkloadByParentUid(ctx, workloadUID)
		if err != nil {
			log.Warnf("Failed to query child workloads for %s: %v", workloadUID, err)
		} else if len(childWorkloads) > 0 {
			for _, child := range childWorkloads {
				childPod, err := p.SelectTargetPod(ctx, child.UID)
				if err == nil && childPod != nil {
					return childPod, nil
				}
			}
		}
	}

	if len(pods) == 0 {
		return nil, fmt.Errorf("no pods found for workload %s", workloadUID)
	}

	// Prioritize pods ending with master-0
	for _, pod := range pods {
		if strings.HasSuffix(pod.Name, "master-0") {
			log.Infof("Selected master-0 pod: %s/%s for workload %s", pod.Namespace, pod.Name, workloadUID)
			return pod, nil
		}
	}

	// If no master-0, return first pod
	selectedPod := pods[0]
	log.Infof("No master-0 pod found, selected first pod: %s/%s for workload %s",
		selectedPod.Namespace, selectedPod.Name, workloadUID)
	return selectedPod, nil
}

// GetNodeExporterClient gets the node-exporter client for a given node
func (p *PodProber) GetNodeExporterClient(ctx context.Context, nodeName string) (*client.Client, error) {
	if p.collector == nil {
		return nil, fmt.Errorf("metadata collector is not initialized")
	}
	return p.collector.GetNodeExporterClientForPod(ctx, nodeName)
}

// GetProcessTree gets the process tree for a pod
func (p *PodProber) GetProcessTree(ctx context.Context, pod *model.GpuPods, opts ProcessTreeOptions) (*types.PodProcessTree, error) {
	nodeClient, err := p.GetNodeExporterClient(ctx, pod.NodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get node-exporter client: %w", err)
	}

	req := &types.ProcessTreeRequest{
		PodName:          pod.Name,
		PodNamespace:     pod.Namespace,
		PodUID:           pod.UID,
		IncludeEnv:       opts.IncludeEnv,
		IncludeCmdline:   opts.IncludeCmdline,
		IncludeResources: opts.IncludeResources,
	}

	return nodeClient.GetPodProcessTree(ctx, req)
}

// FindPythonProcess finds the first Python process in the process tree
func (p *PodProber) FindPythonProcess(tree *types.PodProcessTree) *types.ProcessInfo {
	if tree == nil {
		return nil
	}

	for _, container := range tree.Containers {
		if container.RootProcess != nil {
			proc := p.findPythonProcessInTree(container.RootProcess)
			if proc != nil {
				return proc
			}
		}
	}

	return nil
}

// findPythonProcessInTree recursively searches for a Python process
func (p *PodProber) findPythonProcessInTree(proc *types.ProcessInfo) *types.ProcessInfo {
	if proc == nil {
		return nil
	}

	if proc.IsPython {
		return proc
	}

	for _, child := range proc.Children {
		if result := p.findPythonProcessInTree(child); result != nil {
			return result
		}
	}

	return nil
}

// ExtractEnvMap extracts environment variables from a process info to a map
func (p *PodProber) ExtractEnvMap(proc *types.ProcessInfo) map[string]string {
	env := make(map[string]string)
	if proc == nil {
		return env
	}

	for _, envVar := range proc.Env {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}

	return env
}

// ReadContainerFile reads a file from container filesystem
func (p *PodProber) ReadContainerFile(ctx context.Context, nodeName string, pid int, path string) (string, error) {
	nodeClient, err := p.GetNodeExporterClient(ctx, nodeName)
	if err != nil {
		return "", fmt.Errorf("failed to get node-exporter client: %w", err)
	}

	req := &types.ContainerFileReadRequest{
		PID:            pid,
		Path:           path,
		FollowSymlinks: true,
	}

	resp, err := nodeClient.ReadContainerFile(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

// IsPodReady checks if a pod is in ready state
func (p *PodProber) IsPodReady(ctx context.Context, pod *model.GpuPods) bool {
	if pod == nil {
		return false
	}

	// Check if pod phase is "Running"
	if pod.Phase != "Running" {
		return false
	}

	return true
}

// GetPodAge returns how long the pod has been running
func (p *PodProber) GetPodAge(ctx context.Context, pod *model.GpuPods) time.Duration {
	if pod == nil || pod.CreatedAt.IsZero() {
		return 0
	}
	return time.Since(pod.CreatedAt)
}

// GetPodByUID gets a pod by its UID
func (p *PodProber) GetPodByUID(ctx context.Context, podUID string) (*model.GpuPods, error) {
	return p.podFacade.GetGpuPodsByPodUid(ctx, podUID)
}

// ListPodsByWorkload lists all pods for a workload
func (p *PodProber) ListPodsByWorkload(ctx context.Context, workloadUID string) ([]*model.GpuPods, error) {
	podRefs, err := p.workloadFacade.ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUID)
	if err != nil {
		return nil, err
	}

	if len(podRefs) == 0 {
		return nil, nil
	}

	podUIDs := make([]string, 0, len(podRefs))
	for _, ref := range podRefs {
		podUIDs = append(podUIDs, ref.PodUID)
	}

	var pods []*model.GpuPods
	db := database.GetFacade().GetSystemConfig().GetDB()
	err = db.WithContext(ctx).
		Where("uid IN ? AND deleted = ?", podUIDs, false).
		Find(&pods).Error
	if err != nil {
		return nil, err
	}

	return pods, nil
}

// GetNodeByName gets a node by its name
func (p *PodProber) GetNodeByName(ctx context.Context, nodeName string) (*model.Node, error) {
	return p.nodeFacade.GetNodeByName(ctx, nodeName)
}

// ProbeProcessInfo probes process information from a pod and returns the Python process info
func (p *PodProber) ProbeProcessInfo(ctx context.Context, workloadUID string) (*types.ProcessInfo, *model.GpuPods, error) {
	// Select target pod
	pod, err := p.SelectTargetPod(ctx, workloadUID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to select target pod: %w", err)
	}

	// Check pod readiness
	if !p.IsPodReady(ctx, pod) {
		return nil, pod, fmt.Errorf("pod %s/%s is not ready", pod.Namespace, pod.Name)
	}

	// Get process tree
	tree, err := p.GetProcessTree(ctx, pod, DefaultProcessTreeOptions())
	if err != nil {
		return nil, pod, fmt.Errorf("failed to get process tree: %w", err)
	}

	// Find Python process
	pythonProc := p.FindPythonProcess(tree)
	if pythonProc == nil {
		return nil, pod, fmt.Errorf("no Python process found in pod %s/%s", pod.Namespace, pod.Name)
	}

	// Remove children to reduce data size
	pythonProc.Children = nil

	return pythonProc, pod, nil
}
