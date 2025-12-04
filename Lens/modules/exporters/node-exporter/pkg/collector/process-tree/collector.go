package processtree

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

var (
	collectorInstance *Collector
	once              sync.Once
)

// Collector collects process tree information
type Collector struct {
	procReader      *ProcReader
	containerReader *ContainerdReader
	kubeletReader   *KubeletReader
	nsenterExecutor *NsenterExecutor

	cache    sync.Map // podUID -> *PodProcessTree
	cacheTTL time.Duration
}

// InitCollector initializes the process tree collector
func InitCollector(ctx context.Context) error {
	var initErr error
	once.Do(func() {
		collectorInstance = &Collector{
			procReader: NewProcReader(),
			cacheTTL:   5 * time.Minute,
		}

		// Initialize containerd reader
		var err error
		collectorInstance.containerReader, err = NewContainerdReader()
		if err != nil {
			log.Warnf("Failed to initialize containerd reader: %v", err)
		}

		// Initialize kubelet reader
		collectorInstance.kubeletReader, err = NewKubeletReader()
		if err != nil {
			log.Warnf("Failed to initialize kubelet reader: %v", err)
		}

		// Initialize nsenter executor
		collectorInstance.nsenterExecutor = NewNsenterExecutor()

		log.Info("Process tree collector initialized")
	})
	return initErr
}

// GetCollector returns the global collector instance
func GetCollector() *Collector {
	return collectorInstance
}

// GetPodProcessTree retrieves the complete process tree for a pod
func (c *Collector) GetPodProcessTree(ctx context.Context, req *ProcessTreeRequest) (*PodProcessTree, error) {
	// Check cache
	if cached, ok := c.cache.Load(req.PodUID); ok {
		tree := cached.(*PodProcessTree)
		if time.Since(tree.CollectedAt) < c.cacheTTL {
			log.Debugf("Using cached process tree for pod %s", req.PodUID)
			return tree, nil
		}
	}

	// Build process tree
	tree, err := c.buildPodProcessTree(ctx, req)
	if err != nil {
		return nil, err
	}

	// Update cache
	c.cache.Store(req.PodUID, tree)

	return tree, nil
}

// buildPodProcessTree builds the complete process tree
func (c *Collector) buildPodProcessTree(ctx context.Context, req *ProcessTreeRequest) (*PodProcessTree, error) {
	tree := &PodProcessTree{
		PodName:      req.PodName,
		PodNamespace: req.PodNamespace,
		PodUID:       req.PodUID,
		CollectedAt:  time.Now(),
	}

	// Step 1: Get pod information from kubelet (optional, for additional metadata)
	if c.kubeletReader != nil {
		_, err := c.kubeletReader.GetPodInfo(ctx, req.PodNamespace, req.PodName)
		if err != nil {
			log.Warnf("Failed to get pod info from kubelet: %v", err)
		}
	}

	var err error

	// Step 2: Get container information from containerd
	var containers []*ContainerInfo
	if c.containerReader != nil {
		containers, err = c.containerReader.GetPodContainers(ctx, req.PodUID)
		if err != nil {
			log.Warnf("Failed to get containers from containerd: %v", err)
		}
	}

	// Step 3: Scan processes from /proc
	if len(containers) == 0 {
		// Fallback: find containers by scanning /proc
		containers = c.procReader.FindPodContainersByUID(req.PodUID)
	}

	if len(containers) == 0 {
		return nil, fmt.Errorf("no containers found for pod %s/%s", req.PodNamespace, req.PodName)
	}

	// Step 4: Build process tree for each container
	for _, container := range containers {
		containerTree, err := c.buildContainerProcessTree(ctx, container, req)
		if err != nil {
			log.Warnf("Failed to build process tree for container %s: %v", container.ID, err)
			continue
		}

		tree.Containers = append(tree.Containers, containerTree)
		tree.TotalProcesses += containerTree.ProcessCount
		tree.TotalPython += containerTree.PythonCount
	}

	if len(tree.Containers) == 0 {
		return nil, fmt.Errorf("failed to build process trees for pod %s/%s", req.PodNamespace, req.PodName)
	}

	return tree, nil
}

// buildContainerProcessTree builds process tree for a single container
func (c *Collector) buildContainerProcessTree(ctx context.Context, container *ContainerInfo, req *ProcessTreeRequest) (*ContainerProcessTree, error) {
	// Find all processes in the container
	pids := c.procReader.FindContainerProcesses(container.ID)
	if len(pids) == 0 {
		return nil, fmt.Errorf("no processes found for container %s", container.ID)
	}

	// Build process information for each PID
	processMap := make(map[int]*ProcessInfo)
	pythonCount := 0

	for _, pid := range pids {
		procInfo, err := c.procReader.GetProcessInfo(pid, req)
		if err != nil {
			log.Debugf("Failed to get process info for PID %d: %v", pid, err)
			continue
		}

		// Enrich with container information
		procInfo.ContainerID = container.ID
		procInfo.ContainerName = container.Name
		procInfo.PodUID = req.PodUID
		procInfo.PodName = req.PodName
		procInfo.PodNamespace = req.PodNamespace

		// Get container PID using nsenter
		if c.nsenterExecutor != nil {
			containerPID, err := c.nsenterExecutor.GetContainerPID(pid)
			if err == nil {
				procInfo.ContainerPID = containerPID
			}
		}

		processMap[pid] = procInfo

		if procInfo.IsPython {
			pythonCount++
		}
	}

	// Build tree structure
	var rootProcess *ProcessInfo
	for pid, proc := range processMap {
		// Find parent
		if proc.HostPPID != 0 && proc.HostPPID != pid {
			if parent, ok := processMap[proc.HostPPID]; ok {
				parent.Children = append(parent.Children, proc)
			} else {
				// Parent not in container, this is a root process
				if rootProcess == nil || pid < rootProcess.HostPID {
					rootProcess = proc
				}
			}
		} else {
			// No parent or self-parent, this is root
			if rootProcess == nil || pid < rootProcess.HostPID {
				rootProcess = proc
			}
		}
	}

	// Convert map to slice
	allProcesses := make([]*ProcessInfo, 0, len(processMap))
	for _, proc := range processMap {
		allProcesses = append(allProcesses, proc)
	}

	return &ContainerProcessTree{
		ContainerID:   container.ID,
		ContainerName: container.Name,
		ImageName:     container.Image,
		RootProcess:   rootProcess,
		AllProcesses:  allProcesses,
		ProcessCount:  len(allProcesses),
		PythonCount:   pythonCount,
	}, nil
}

// FindPythonProcesses finds all Python processes in a pod
func (c *Collector) FindPythonProcesses(ctx context.Context, podUID string) ([]*ProcessInfo, error) {
	req := &ProcessTreeRequest{
		PodUID:         podUID,
		IncludeCmdline: true,
	}

	tree, err := c.GetPodProcessTree(ctx, req)
	if err != nil {
		return nil, err
	}

	var pythonProcesses []*ProcessInfo
	for _, container := range tree.Containers {
		for _, proc := range container.AllProcesses {
			if proc.IsPython {
				pythonProcesses = append(pythonProcesses, proc)
			}
		}
	}

	return pythonProcesses, nil
}

// InvalidateCache invalidates the cache for a pod
func (c *Collector) InvalidateCache(podUID string) {
	c.cache.Delete(podUID)
}
