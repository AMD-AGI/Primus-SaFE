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

	cache    sync.Map // cacheKey -> *PodProcessTree
	cacheTTL time.Duration
}

// cacheKey represents a unique cache key including request parameters
type cacheKey struct {
	PodUID           string
	IncludeEnv       bool
	IncludeCmdline   bool
	IncludeResources bool
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
	// Create cache key with request parameters
	key := cacheKey{
		PodUID:           req.PodUID,
		IncludeEnv:       req.IncludeEnv,
		IncludeCmdline:   req.IncludeCmdline,
		IncludeResources: req.IncludeResources,
	}

	// Check cache
	if cached, ok := c.cache.Load(key); ok {
		tree := cached.(*PodProcessTree)
		if time.Since(tree.CollectedAt) < c.cacheTTL {
			log.Debugf("Using cached process tree for pod %s (env:%v, cmdline:%v, resources:%v)",
				req.PodUID, req.IncludeEnv, req.IncludeCmdline, req.IncludeResources)
			return tree, nil
		}
	}

	// Build process tree
	tree, err := c.buildPodProcessTree(ctx, req)
	if err != nil {
		return nil, err
	}

	// Update cache
	c.cache.Store(key, tree)

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

// InvalidateCache invalidates all cache entries for a pod
func (c *Collector) InvalidateCache(podUID string) {
	// Delete all cache entries for this pod (with different parameter combinations)
	c.cache.Range(func(key, value interface{}) bool {
		if k, ok := key.(cacheKey); ok && k.PodUID == podUID {
			c.cache.Delete(key)
		}
		return true
	})
}

// GetProcessEnvironment gets environment variables for processes in a pod
func (c *Collector) GetProcessEnvironment(ctx context.Context, req *ProcessEnvRequest) (*ProcessEnvResponse, error) {
	// Find all containers in the pod
	var containers []*ContainerInfo
	if c.containerReader != nil {
		var err error
		containers, err = c.containerReader.GetPodContainers(ctx, req.PodUID)
		if err != nil {
			log.Warnf("Failed to get containers from containerd: %v", err)
		}
	}

	// Fallback: find containers by scanning /proc
	if len(containers) == 0 {
		containers = c.procReader.FindPodContainersByUID(req.PodUID)
	}

	if len(containers) == 0 {
		return nil, fmt.Errorf("no containers found for pod UID %s", req.PodUID)
	}

	// Collect all PIDs from all containers
	var allPids []int
	for _, container := range containers {
		pids := c.procReader.FindContainerProcesses(container.ID)
		allPids = append(allPids, pids...)
	}

	if len(allPids) == 0 {
		return nil, fmt.Errorf("no processes found for pod UID %s", req.PodUID)
	}

	// If specific PID is requested, filter
	if req.PID != 0 {
		found := false
		for _, pid := range allPids {
			if pid == req.PID {
				allPids = []int{req.PID}
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("PID %d not found in pod", req.PID)
		}
	}

	// Get environment variables for each process
	var processEnvs []*ProcessEnvInfo
	for _, pid := range allPids {
		env, cmdline, err := c.procReader.GetProcessEnvironment(pid, req.FilterPrefix)
		if err != nil {
			log.Debugf("Failed to get environment for PID %d: %v", pid, err)
			continue
		}

		processEnvs = append(processEnvs, &ProcessEnvInfo{
			PID:         pid,
			Cmdline:     cmdline,
			Environment: env,
		})
	}

	return &ProcessEnvResponse{
		PodUID:    req.PodUID,
		Processes: processEnvs,
		Collected: time.Now(),
	}, nil
}

// GetProcessArguments gets command line arguments for processes in a pod
func (c *Collector) GetProcessArguments(ctx context.Context, req *ProcessArgsRequest) (*ProcessArgsResponse, error) {
	// Find all containers in the pod
	var containers []*ContainerInfo
	if c.containerReader != nil {
		var err error
		containers, err = c.containerReader.GetPodContainers(ctx, req.PodUID)
		if err != nil {
			log.Warnf("Failed to get containers from containerd: %v", err)
		}
	}

	// Fallback: find containers by scanning /proc
	if len(containers) == 0 {
		containers = c.procReader.FindPodContainersByUID(req.PodUID)
	}

	if len(containers) == 0 {
		return nil, fmt.Errorf("no containers found for pod UID %s", req.PodUID)
	}

	// Collect all PIDs from all containers
	var allPids []int
	for _, container := range containers {
		pids := c.procReader.FindContainerProcesses(container.ID)
		allPids = append(allPids, pids...)
	}

	if len(allPids) == 0 {
		return nil, fmt.Errorf("no processes found for pod UID %s", req.PodUID)
	}

	// If specific PID is requested, filter
	if req.PID != 0 {
		found := false
		for _, pid := range allPids {
			if pid == req.PID {
				allPids = []int{req.PID}
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("PID %d not found in pod", req.PID)
		}
	}

	// Get arguments for each process
	var processArgs []*ProcessArgInfo
	for _, pid := range allPids {
		cmdline, args, err := c.procReader.GetProcessArguments(pid)
		if err != nil {
			log.Debugf("Failed to get arguments for PID %d: %v", pid, err)
			continue
		}

		processArgs = append(processArgs, &ProcessArgInfo{
			PID:     pid,
			Cmdline: cmdline,
			Args:    args,
		})
	}

	return &ProcessArgsResponse{
		PodUID:    req.PodUID,
		Processes: processArgs,
		Collected: time.Now(),
	}, nil
}

// FindTensorboardFiles finds all tensorboard event files opened by processes in a pod
func (c *Collector) FindTensorboardFiles(ctx context.Context, podUID, podName, podNamespace string) (*TensorboardFilesResponse, error) {
	// Find all containers in the pod
	var containers []*ContainerInfo
	if c.containerReader != nil {
		var err error
		containers, err = c.containerReader.GetPodContainers(ctx, podUID)
		if err != nil {
			log.Warnf("Failed to get containers from containerd: %v", err)
		}
	}

	// Fallback: find containers by scanning /proc
	if len(containers) == 0 {
		containers = c.procReader.FindPodContainersByUID(podUID)
	}

	if len(containers) == 0 {
		return nil, fmt.Errorf("no containers found for pod UID %s", podUID)
	}

	// Collect all PIDs from all containers
	var allPids []int
	for _, container := range containers {
		pids := c.procReader.FindContainerProcesses(container.ID)
		allPids = append(allPids, pids...)
	}

	if len(allPids) == 0 {
		return nil, fmt.Errorf("no processes found for pod UID %s", podUID)
	}

	// Scan for tensorboard files
	tensorboardFiles := c.procReader.ScanTensorboardFiles(allPids)

	return &TensorboardFilesResponse{
		PodUID:         podUID,
		PodName:        podName,
		PodNamespace:   podNamespace,
		Files:          tensorboardFiles,
		TotalProcesses: len(allPids),
		CollectedAt:    time.Now(),
	}, nil
}

// FindPyTorchProfilerFiles finds all PyTorch Profiler files opened by processes in a pod
func (c *Collector) FindPyTorchProfilerFiles(ctx context.Context, podUID, podName, podNamespace string) (*PyTorchProfilerFilesResponse, error) {
	// Find all containers in the pod
	var containers []*ContainerInfo
	if c.containerReader != nil {
		var err error
		containers, err = c.containerReader.GetPodContainers(ctx, podUID)
		if err != nil {
			log.Warnf("Failed to get containers from containerd: %v", err)
		}
	}

	// Fallback: find containers by scanning /proc
	if len(containers) == 0 {
		containers = c.procReader.FindPodContainersByUID(podUID)
	}

	if len(containers) == 0 {
		return nil, fmt.Errorf("no containers found for pod UID %s", podUID)
	}

	// Collect all PIDs from all containers
	var allPids []int
	for _, container := range containers {
		pids := c.procReader.FindContainerProcesses(container.ID)
		allPids = append(allPids, pids...)
	}

	if len(allPids) == 0 {
		return nil, fmt.Errorf("no processes found for pod UID %s", podUID)
	}

	// Scan for PyTorch Profiler files
	profilerFiles := c.procReader.ScanPyTorchProfilerFiles(allPids)

	return &PyTorchProfilerFilesResponse{
		PodUID:         podUID,
		PodName:        podName,
		PodNamespace:   podNamespace,
		Files:          profilerFiles,
		TotalProcesses: len(allPids),
		CollectedAt:    time.Now(),
	}, nil
}
