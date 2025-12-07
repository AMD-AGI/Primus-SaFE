package metadata

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/client"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/types"
)

var (
	collectorInstance *Collector
	once              sync.Once
)

// Collector collects and stores workload metadata
type Collector struct {
	storage  Storage
	cache    sync.Map // workloadUID -> *CollectionResult
	cacheTTL time.Duration

	// Node-specific client cache
	clientCache sync.Map // nodeName -> *client.Client

	// Node Exporter configuration
	nodeExporterPort int

	// Database facades for node and pod info
	nodeFacade      database.NodeFacadeInterface
	podFacade       database.PodFacadeInterface
	detectionFacade database.AiWorkloadMetadataFacadeInterface
}

// Storage defines the interface for metadata storage
type Storage interface {
	Store(ctx context.Context, metadata *WorkloadMetadata) error
	Get(ctx context.Context, workloadUID string) (*WorkloadMetadata, error)
	Query(ctx context.Context, query *MetadataQuery) ([]*WorkloadMetadata, error)
	Delete(ctx context.Context, workloadUID string) error
}

// InitCollector initializes the metadata collector
func InitCollector(ctx context.Context, storage Storage) error {
	var initErr error
	once.Do(func() {
		collectorInstance = &Collector{
			storage:          storage,
			cacheTTL:         10 * time.Minute,
			nodeExporterPort: 8989, // Default node-exporter port
			nodeFacade:       database.NewNodeFacade(),
			podFacade:        database.NewPodFacade(),
			detectionFacade:  database.NewAiWorkloadMetadataFacade(),
		}
		log.Info("Metadata collector initialized")
	})
	return initErr
}

// GetCollector returns the global collector instance
func GetCollector() *Collector {
	return collectorInstance
}

// CollectMetadata collects complete metadata for a training workload
func (c *Collector) CollectMetadata(ctx context.Context, req *CollectionRequest) (*CollectionResult, error) {
	startTime := time.Now()

	log.Infof("Starting metadata collection for workload %s (pod %s/%s)",
		req.WorkloadUID, req.PodNamespace, req.PodName)

	// Check cache unless forced
	if !req.Force {
		if cached, ok := c.cache.Load(req.WorkloadUID); ok {
			result := cached.(*CollectionResult)
			if time.Since(time.Now().Add(-time.Duration(result.Duration)*time.Second)) < c.cacheTTL {
				log.Debugf("Using cached metadata for workload %s", req.WorkloadUID)
				return result, nil
			}
		}
	}

	result := &CollectionResult{
		Success: false,
	}

	// Get node name and IP from database
	nodeName, nodeIP, err := c.getNodeInfoFromDB(ctx, req.PodUID)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get node info from database: %v", err)
		result.Duration = time.Since(startTime).Seconds()
		return result, err
	}

	log.Infof("Pod %s/%s is on node %s (IP: %s)", req.PodNamespace, req.PodName, nodeName, nodeIP)

	// Get node-specific client
	client, err := c.getNodeExporterClientByIP(nodeIP, nodeName)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get node-exporter client: %v", err)
		result.Duration = time.Since(startTime).Seconds()
		return result, err
	}

	// Step 1: Get process tree
	log.Debugf("Getting process tree for pod %s on node %s", req.PodUID, req.NodeName)
	processTree, err := client.GetPodProcessTree(ctx, &types.ProcessTreeRequest{
		PodName:          req.PodName,
		PodNamespace:     req.PodNamespace,
		PodUID:           req.PodUID,
		IncludeCmdline:   true,
		IncludeEnv:       false, // We'll get env from inspection if needed
		IncludeResources: true,
	})

	if err != nil {
		result.Error = fmt.Sprintf("failed to get process tree: %v", err)
		result.Duration = time.Since(startTime).Seconds()
		return result, err
	}

	result.ProcessCount = processTree.TotalProcesses
	result.PythonCount = processTree.TotalPython

	if processTree.TotalPython == 0 {
		result.Error = "no Python processes found in pod"
		result.Duration = time.Since(startTime).Seconds()
		return result, fmt.Errorf(result.Error)
	}

	log.Infof("Found %d Python processes in pod %s", processTree.TotalPython, req.PodName)

	// Step 2: Find root Python processes from process tree
	rootPythonProcesses := c.findRootPythonProcesses(processTree)
	if len(rootPythonProcesses) == 0 {
		result.Error = "no root Python processes found in pod"
		result.Duration = time.Since(startTime).Seconds()
		return result, fmt.Errorf(result.Error)
	}

	log.Infof("Found %d root Python processes in pod %s", len(rootPythonProcesses), req.PodName)

	// Step 3: Determine which scripts to run based on detection results
	scripts := req.Scripts
	if len(scripts) == 0 {
		// Get scripts from node-exporter and match with detection
		scripts, err = c.selectScriptsFromDetection(ctx, client, req.WorkloadUID)
		if err != nil {
			log.Warnf("Failed to select scripts from detection: %v", err)
			// Fallback: use minimal universal scripts
			scripts = []string{"tensorboard"}
		}
		if len(scripts) == 0 {
			scripts = []string{"tensorboard"} // Fallback to universal
		}
	}

	log.Infof("Will run inspection scripts: %v", scripts)

	// Step 4: Inspect Python processes
	timeout := req.Timeout
	if timeout == 0 {
		timeout = 30 // 30 seconds default
	}

	metadata := &WorkloadMetadata{
		WorkloadUID:      req.WorkloadUID,
		PodName:          req.PodName,
		PodNamespace:     req.PodNamespace,
		NodeName:         nodeName,
		Frameworks:       []string{},
		CollectedAt:      time.Now(),
		CollectionSource: "node-exporter",
		Confidence:       0.0,
	}

	inspectedCount := 0
	confidenceSum := 0.0

	// Inspect only root Python processes (training main processes)
	for _, proc := range rootPythonProcesses {
		log.Infof("Inspecting root Python process PID=%d, cmdline=%s", proc.HostPID, proc.Cmdline)

		inspectionResults, err := client.InspectPythonProcess(
			ctx, proc.HostPID, scripts, timeout,
		)

		if err != nil {
			log.Warnf("Failed to inspect process %d: %v", proc.HostPID, err)
			// Try next root process if available
			continue
		}

		// Parse inspection results
		foundValidData := false
		for scriptName, inspResult := range inspectionResults {
			if !inspResult.Success {
				log.Debugf("Inspection script %s failed for PID %d: %s",
					scriptName, proc.HostPID, inspResult.Error)
				continue
			}

			log.Infof("Inspection successful: script=%s, pid=%d, duration=%.2fs",
				inspResult.Script, inspResult.PID, inspResult.Duration)
			inspectedCount++
			foundValidData = true

			// Parse script-specific data
			switch scriptName {
			case "pytorch":
				if pytorchData, err := parsePyTorchData(inspResult); err == nil {
					metadata.PyTorchInfo = pytorchData
					if !contains(metadata.Frameworks, "pytorch") {
						metadata.Frameworks = append(metadata.Frameworks, "pytorch")
					}
					metadata.BaseFramework = "pytorch"
					confidenceSum += 0.9 // High confidence for direct inspection
				}

			case "tensorboard":
				if tbData, err := parseTensorBoardData(inspResult); err == nil {
					metadata.TensorBoardInfo = tbData
					confidenceSum += 0.8
				}

			case "megatron":
				if megatronData, err := parseMegatronData(inspResult); err == nil {
					metadata.MegatronInfo = megatronData
					if !contains(metadata.Frameworks, "megatron") {
						metadata.Frameworks = append(metadata.Frameworks, "megatron")
					}
					metadata.WrapperFramework = "megatron"
					confidenceSum += 0.9
				}
			}
		}

		// If we found valid data from this root process, we can stop
		// (usually there's only one main training process)
		if foundValidData {
			log.Infof("Successfully collected data from root process PID=%d", proc.HostPID)
			break
		}
	}

	// Calculate overall confidence
	if inspectedCount > 0 {
		metadata.Confidence = confidenceSum / float64(inspectedCount)
	}

	// Detect Primus by checking cmdline patterns from root processes
	if primusData := c.detectPrimusFromProcesses(rootPythonProcesses); primusData != nil {
		metadata.PrimusInfo = primusData
		if !contains(metadata.Frameworks, "primus") {
			metadata.Frameworks = append(metadata.Frameworks, "primus")
		}
		if metadata.WrapperFramework == "" {
			metadata.WrapperFramework = "primus"
		}
	}

	// Store metadata
	if err := c.storage.Store(ctx, metadata); err != nil {
		log.Errorf("Failed to store metadata: %v", err)
		result.Error = fmt.Sprintf("failed to store metadata: %v", err)
	} else {
		result.Success = true
		result.Metadata = metadata
	}

	result.InspectedCount = inspectedCount
	result.Duration = time.Since(startTime).Seconds()

	// Cache result
	c.cache.Store(req.WorkloadUID, result)

	log.Infof("Metadata collection completed for workload %s: success=%v, frameworks=%v, duration=%.2fs",
		req.WorkloadUID, result.Success, metadata.Frameworks, result.Duration)

	return result, nil
}

// parsePyTorchData parses PyTorch inspection data
func parsePyTorchData(result *types.InspectionResult) (*PyTorchMetadata, error) {
	data := result.Data

	pytorch := &PyTorchMetadata{}

	if version, ok := data["version"].(string); ok {
		pytorch.Version = version
	}

	if cudaAvail, ok := data["cuda_available"].(bool); ok {
		pytorch.CudaAvailable = cudaAvail
	}

	if cudaVer, ok := data["cuda_version"].(string); ok {
		pytorch.CudaVersion = cudaVer
	}

	if device, ok := data["device"].(string); ok {
		pytorch.Device = device
	}

	if distMode, ok := data["distributed_mode"].(string); ok {
		pytorch.DistributedMode = distMode
	}

	if mixedPrec, ok := data["mixed_precision"].(bool); ok {
		pytorch.MixedPrecision = mixedPrec
	}

	// Parse models
	if modelsData, ok := data["models"].([]interface{}); ok {
		for _, modelData := range modelsData {
			if modelMap, ok := modelData.(map[string]interface{}); ok {
				model := ModelInfo{}
				if name, ok := modelMap["name"].(string); ok {
					model.Name = name
				}
				if modelType, ok := modelMap["type"].(string); ok {
					model.Type = modelType
				}
				if params, ok := modelMap["total_params"].(float64); ok {
					model.Parameters = int64(params)
					pytorch.TotalParams += int64(params)
				}
				if trainable, ok := modelMap["trainable_params"].(float64); ok {
					model.TrainableParams = int64(trainable)
					pytorch.TrainableParams += int64(trainable)
				}
				if device, ok := modelMap["device"].(string); ok {
					model.Device = device
				}
				pytorch.Models = append(pytorch.Models, model)
			}
		}
	}

	return pytorch, nil
}

// parseTensorBoardData parses TensorBoard inspection data
func parseTensorBoardData(result *types.InspectionResult) (*TensorBoardMetadata, error) {
	data := result.Data

	tb := &TensorBoardMetadata{
		Enabled: true, // If we got data, TensorBoard is enabled
	}

	if logDir, ok := data["log_dir"].(string); ok {
		tb.LogDir = logDir
	}

	if port, ok := data["port"].(float64); ok {
		tb.Port = int(port)
	}

	if writers, ok := data["writers"].([]interface{}); ok {
		for _, w := range writers {
			if writerStr, ok := w.(string); ok {
				tb.Writers = append(tb.Writers, writerStr)
			}
		}
	}

	if updateFreq, ok := data["update_freq"].(string); ok {
		tb.UpdateFreq = updateFreq
	}

	return tb, nil
}

// parseMegatronData parses Megatron-LM inspection data
func parseMegatronData(result *types.InspectionResult) (*MegatronMetadata, error) {
	data := result.Data

	megatron := &MegatronMetadata{}

	if version, ok := data["version"].(string); ok {
		megatron.Version = version
	}

	if tp, ok := data["tensor_parallel"].(float64); ok {
		megatron.TensorParallel = int(tp)
	}

	if pp, ok := data["pipeline_parallel"].(float64); ok {
		megatron.PipelineParallel = int(pp)
	}

	if dp, ok := data["data_parallel"].(float64); ok {
		megatron.DataParallel = int(dp)
	}

	if sp, ok := data["sequence_parallel"].(bool); ok {
		megatron.SequenceParallel = sp
	}

	if mbs, ok := data["micro_batch_size"].(float64); ok {
		megatron.MicroBatchSize = int(mbs)
	}

	if gbs, ok := data["global_batch_size"].(float64); ok {
		megatron.GlobalBatchSize = int(gbs)
	}

	if seqLen, ok := data["sequence_length"].(float64); ok {
		megatron.SequenceLength = int(seqLen)
	}

	if hiddenSize, ok := data["hidden_size"].(float64); ok {
		megatron.HiddenSize = int(hiddenSize)
	}

	if numLayers, ok := data["num_layers"].(float64); ok {
		megatron.NumLayers = int(numLayers)
	}

	if numHeads, ok := data["num_attention_heads"].(float64); ok {
		megatron.NumAttentionHeads = int(numHeads)
	}

	if vocabSize, ok := data["vocab_size"].(float64); ok {
		megatron.VocabSize = int(vocabSize)
	}

	if lr, ok := data["learning_rate"].(float64); ok {
		megatron.LearningRate = lr
	}

	if opt, ok := data["optimizer"].(string); ok {
		megatron.Optimizer = opt
	}

	return megatron, nil
}

// detectPrimusFromProcesses detects Primus wrapper by analyzing process cmdlines
func (c *Collector) detectPrimusFromProcesses(processes []*types.ProcessInfo) *PrimusMetadata {
	for _, proc := range processes {
		cmdline := strings.ToLower(proc.Cmdline)

		// Look for Primus indicators in command line
		if strings.Contains(cmdline, "primus") ||
			strings.Contains(cmdline, "primus-train") ||
			strings.Contains(cmdline, "primus.") {

			primus := &PrimusMetadata{
				Mode:     "training",
				Features: []string{},
			}

			// Try to detect version and configuration from cmdline
			if strings.Contains(cmdline, "--") {
				// Parse command line flags
				parts := strings.Split(cmdline, "--")
				for _, part := range parts[1:] {
					trimmed := strings.TrimSpace(part)
					if strings.HasPrefix(trimmed, "version") {
						// Extract version
						fields := strings.Fields(trimmed)
						if len(fields) > 1 {
							primus.Version = fields[1]
						}
					}
				}
			}

			log.Infof("Detected Primus wrapper framework in process %d", proc.HostPID)
			return primus
		}
	}

	return nil
}

// GetMetadata retrieves stored metadata for a workload
func (c *Collector) GetMetadata(ctx context.Context, workloadUID string) (*WorkloadMetadata, error) {
	return c.storage.Get(ctx, workloadUID)
}

// QueryMetadata queries stored metadata with filters
func (c *Collector) QueryMetadata(ctx context.Context, query *MetadataQuery) ([]*WorkloadMetadata, error) {
	return c.storage.Query(ctx, query)
}

// InvalidateCache invalidates the cache for a workload
func (c *Collector) InvalidateCache(workloadUID string) {
	c.cache.Delete(workloadUID)
}

// findRootPythonProcesses finds the root (top-level) Python processes from process tree
// These are typically the main training processes
func (c *Collector) findRootPythonProcesses(processTree *types.PodProcessTree) []*types.ProcessInfo {
	var rootProcesses []*types.ProcessInfo

	// Iterate through all containers in the pod
	for _, container := range processTree.Containers {
		// Find Python processes with minimal depth (closest to root)
		pythonProcesses := c.findPythonProcessesInTree(container.RootProcess)

		if len(pythonProcesses) > 0 {
			// Find the Python process with the smallest depth (closest to root)
			minDepth := -1
			var rootPython []*types.ProcessInfo

			for _, proc := range pythonProcesses {
				depth := c.calculateProcessDepth(proc, container.RootProcess)
				if minDepth == -1 || depth < minDepth {
					minDepth = depth
					rootPython = []*types.ProcessInfo{proc}
				} else if depth == minDepth {
					rootPython = append(rootPython, proc)
				}
			}

			rootProcesses = append(rootProcesses, rootPython...)
		}
	}

	if len(rootProcesses) > 0 {
		log.Infof("Found %d root Python processes", len(rootProcesses))
		for _, proc := range rootProcesses {
			log.Debugf("Root Python process: PID=%d, cmdline=%s", proc.HostPID, proc.Cmdline)
		}
	}

	return rootProcesses
}

// findPythonProcessesInTree recursively finds all Python processes in the tree
func (c *Collector) findPythonProcessesInTree(root *types.ProcessInfo) []*types.ProcessInfo {
	var pythonProcesses []*types.ProcessInfo

	if root == nil {
		return pythonProcesses
	}

	// Check if current process is Python
	if root.IsPython {
		pythonProcesses = append(pythonProcesses, root)
	}

	// Recursively check children
	for _, child := range root.Children {
		pythonProcesses = append(pythonProcesses, c.findPythonProcessesInTree(child)...)
	}

	return pythonProcesses
}

// calculateProcessDepth calculates the depth of a process in the tree
func (c *Collector) calculateProcessDepth(target *types.ProcessInfo, root *types.ProcessInfo) int {
	if root == nil {
		return -1
	}

	if root.HostPID == target.HostPID {
		return 0
	}

	for _, child := range root.Children {
		depth := c.calculateProcessDepth(target, child)
		if depth >= 0 {
			return depth + 1
		}
	}

	return -1
}

// selectScriptsFromDetection selects inspection scripts based on detection and available scripts
func (c *Collector) selectScriptsFromDetection(ctx context.Context, nodeExporterClient *client.Client, workloadUID string) ([]string, error) {
	// Step 1: Query detection results from database
	detection, err := c.detectionFacade.GetAiWorkloadMetadata(ctx, workloadUID)
	if err != nil {
		return nil, fmt.Errorf("failed to query detection: %w", err)
	}

	if detection == nil {
		log.Debugf("No detection found for workload %s, will use universal scripts", workloadUID)
		return c.getUniversalScripts(ctx, nodeExporterClient)
	}

	// Step 2: Get available scripts from node-exporter
	availableScripts, err := nodeExporterClient.ListAvailableScripts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list available scripts: %w", err)
	}

	if len(availableScripts) == 0 {
		log.Warn("No inspection scripts available from node-exporter")
		return nil, nil
	}

	log.Debugf("Node-exporter provides %d inspection scripts", len(availableScripts))

	// Step 3: Extract frameworks from detection
	frameworks := c.extractFrameworksFromDetection(detection)
	log.Infof("Detection shows frameworks: %v for workload %s", frameworks, workloadUID)

	// Step 4: Match scripts with frameworks
	selectedScripts := []string{}
	scriptMap := make(map[string]bool) // Avoid duplicates

	for _, script := range availableScripts {
		// Check if script is applicable
		if c.isScriptApplicable(script, frameworks) {
			if !scriptMap[script.Name] {
				selectedScripts = append(selectedScripts, script.Name)
				scriptMap[script.Name] = true
				log.Debugf("Selected script '%s': frameworks=%v, capabilities=%v",
					script.Name, script.Frameworks, script.Capabilities)
			}
		}
	}

	if len(selectedScripts) == 0 {
		log.Warnf("No applicable scripts found for frameworks %v", frameworks)
		// Fallback to universal scripts
		return c.getUniversalScripts(ctx, nodeExporterClient)
	}

	log.Infof("Selected %d inspection scripts based on detection: %v", len(selectedScripts), selectedScripts)
	return selectedScripts, nil
}

// extractFrameworksFromDetection extracts all frameworks from detection result
func (c *Collector) extractFrameworksFromDetection(detection *model.AiWorkloadMetadata) []string {
	frameworks := []string{}
	frameworkMap := make(map[string]bool)

	// Add primary framework
	if detection.Framework != "" {
		fw := strings.ToLower(detection.Framework)
		frameworks = append(frameworks, fw)
		frameworkMap[fw] = true
	}

	// Parse metadata for additional frameworks
	if len(detection.Metadata) > 0 {
		// Check wrapper framework
		if wrapper, ok := detection.Metadata["wrapper_framework"].(string); ok {
			fw := strings.ToLower(wrapper)
			if !frameworkMap[fw] {
				frameworks = append(frameworks, fw)
				frameworkMap[fw] = true
			}
		}

		// Check base framework
		if base, ok := detection.Metadata["base_framework"].(string); ok {
			fw := strings.ToLower(base)
			if !frameworkMap[fw] {
				frameworks = append(frameworks, fw)
				frameworkMap[fw] = true
			}
		}

		// Check frameworks array
		if fwArray, ok := detection.Metadata["frameworks"].([]interface{}); ok {
			for _, fwInterface := range fwArray {
				if fw, ok := fwInterface.(string); ok {
					fw = strings.ToLower(fw)
					if !frameworkMap[fw] {
						frameworks = append(frameworks, fw)
						frameworkMap[fw] = true
					}
				}
			}
		}
	}

	return frameworks
}

// isScriptApplicable checks if a script is applicable for given frameworks
func (c *Collector) isScriptApplicable(script *types.ScriptMetadata, frameworks []string) bool {
	// Universal scripts (empty frameworks list) are always applicable
	if len(script.Frameworks) == 0 {
		log.Debugf("Script '%s' is universal (category: universal)", script.Name)
		return true
	}

	// Check if any detected framework matches script's frameworks
	for _, detectedFw := range frameworks {
		for _, scriptFw := range script.Frameworks {
			if strings.EqualFold(detectedFw, scriptFw) {
				log.Debugf("Script '%s' matches framework '%s'", script.Name, detectedFw)
				return true
			}
		}
	}

	return false
}

// getUniversalScripts returns only universal scripts (applicable to all frameworks)
func (c *Collector) getUniversalScripts(ctx context.Context, nodeExporterClient *client.Client) ([]string, error) {
	availableScripts, err := nodeExporterClient.ListAvailableScripts(ctx)
	if err != nil {
		return nil, err
	}

	universal := []string{}
	for _, script := range availableScripts {
		// Universal scripts have empty frameworks list
		if len(script.Frameworks) == 0 {
			universal = append(universal, script.Name)
		}
	}

	log.Infof("Selected %d universal scripts: %v", len(universal), universal)
	return universal, nil
}

// contains checks if a string slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// getNodeInfoFromDB retrieves node name and IP from database based on pod UID
func (c *Collector) getNodeInfoFromDB(ctx context.Context, podUID string) (nodeName, nodeIP string, err error) {
	// Query gpu_pods table to get node name
	gpuPod, err := c.podFacade.GetGpuPodsByPodUid(ctx, podUID)
	if err != nil {
		return "", "", fmt.Errorf("failed to query gpu_pods: %w", err)
	}

	if gpuPod == nil {
		return "", "", fmt.Errorf("pod with UID %s not found in database", podUID)
	}

	nodeName = gpuPod.NodeName
	if nodeName == "" {
		return "", "", fmt.Errorf("node name is empty for pod %s", podUID)
	}

	log.Debugf("Found pod %s on node %s from database", podUID, nodeName)

	// Query node table to get node IP
	node, err := c.nodeFacade.GetNodeByName(ctx, nodeName)
	if err != nil {
		return "", "", fmt.Errorf("failed to query node table: %w", err)
	}

	if node == nil {
		return "", "", fmt.Errorf("node %s not found in database", nodeName)
	}

	nodeIP = node.Address
	if nodeIP == "" {
		// Fallback: use nodeName as hostname
		log.Warnf("Node IP is empty for node %s, using node name as hostname", nodeName)
		nodeIP = nodeName
	}

	log.Debugf("Found node %s IP: %s from database", nodeName, nodeIP)
	return nodeName, nodeIP, nil
}

// getNodeExporterClientByIP gets or creates a node-exporter client by finding the pod on the target node
func (c *Collector) getNodeExporterClientByIP(nodeIP, nodeName string) (*client.Client, error) {
	// Use nodeName as cache key
	cacheKey := nodeName

	// Check cache first
	if cached, ok := c.clientCache.Load(cacheKey); ok {
		return cached.(*client.Client), nil
	}

	// Get node-exporter pod on the target node using existing clientsets implementation
	ctx := context.Background()
	k8sClient := clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet.ControllerRuntimeClient

	nodeExporterK8sClient, err := clientsets.GetOrInitNodeExportersClient(ctx, nodeName, k8sClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get node-exporter client for node %s: %w", nodeName, err)
	}

	// Convert to our client type by creating a new client with the baseURL
	// The address from GetOrInitNodeExportersClient already includes http:// and port
	// We need to add /v1 prefix as node-exporter uses core router
	baseURL := nodeExporterK8sClient.GetRestyClient().BaseURL + "/v1"
	nodeExporterClient := client.NewClient(client.DefaultConfig(baseURL))

	// Cache the client
	c.clientCache.Store(cacheKey, nodeExporterClient)

	log.Infof("Created node-exporter client for node %s at %s", nodeName, baseURL)
	return nodeExporterClient, nil
}
