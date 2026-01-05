package api

import (
	"context"
	"math"
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	coreErrors "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// FragmentationAnalysisParams - Query parameters for fragmentation analysis
type FragmentationAnalysisParams struct {
	Cluster string `form:"cluster" binding:"required"`
}

// FragmentationAnalysisResponse - Response for /api/v1/nodes/fragmentation-analysis
type FragmentationAnalysisResponse struct {
	Cluster                   string               `json:"cluster"`
	ClusterFragmentationScore float64              `json:"cluster_fragmentation_score"` // 0-100
	TotalNodes                int                  `json:"total_nodes"`
	NodeFragmentations        []NodeFragmentation  `json:"node_fragmentations"`
	Recommendations           []string             `json:"recommendations"`
	Summary                   FragmentationSummary `json:"summary"`
}

// NodeFragmentation - Node fragmentation information
type NodeFragmentation struct {
	NodeName           string  `json:"node_name"`
	TotalGPUs          int32   `json:"total_gpus"`
	AllocatedGPUs      int32   `json:"allocated_gpus"`
	AvailableGPUs      int32   `json:"available_gpus"`
	FragmentationScore float64 `json:"fragmentation_score"` // 0-100
	Status             string  `json:"status"`              // healthy, fragmented, critical
	Utilization        float64 `json:"utilization"`
}

// FragmentationSummary - Fragmentation summary statistics
type FragmentationSummary struct {
	HealthyNodes    int     `json:"healthy_nodes"`
	FragmentedNodes int     `json:"fragmented_nodes"`
	CriticalNodes   int     `json:"critical_nodes"`
	TotalWastedGPUs int     `json:"total_wasted_gpus"`
	WastePercentage float64 `json:"waste_percentage"`
}

// NodeFragmentationDetail - Detailed fragmentation information for a single node
type NodeFragmentationDetail struct {
	NodeName           string            `json:"node_name"`
	TotalGPUs          int32             `json:"total_gpus"`
	AllocatedGPUs      int32             `json:"allocated_gpus"`
	AvailableGPUs      int32             `json:"available_gpus"`
	FragmentationScore float64           `json:"fragmentation_score"`
	Status             string            `json:"status"`
	AllocationPattern  AllocationPattern `json:"allocation_pattern"`
	RunningPods        []PodAllocation   `json:"running_pods"`
	Recommendations    []string          `json:"recommendations"`
}

// AllocationPattern - GPU allocation pattern information
type AllocationPattern struct {
	FullyAllocatedPods   int  `json:"fully_allocated_pods"`
	PartiallyAllocPods   int  `json:"partially_allocated_pods"`
	GPUSharing           bool `json:"gpu_sharing_enabled"`
	LargestContiguousGPU int  `json:"largest_contiguous_gpu"`
}

// PodAllocation - Pod GPU allocation information
type PodAllocation struct {
	PodName       string `json:"pod_name"`
	Namespace     string `json:"namespace"`
	AllocatedGPUs int32  `json:"allocated_gpus"`
}

// LoadBalanceAnalysisResponse - Response for /api/v1/nodes/load-balance-analysis
type LoadBalanceAnalysisResponse struct {
	Cluster              string           `json:"cluster"`
	LoadBalanceScore     float64          `json:"load_balance_score"` // 0-100, higher is better
	NodeLoadDistribution []NodeLoad       `json:"node_load_distribution"`
	HotspotNodes         []string         `json:"hotspot_nodes"`
	IdleNodes            []string         `json:"idle_nodes"`
	Recommendations      []string         `json:"recommendations"`
	Statistics           LoadBalanceStats `json:"statistics"`
}

// NodeLoad - Node load information
type NodeLoad struct {
	NodeName        string  `json:"node_name"`
	AllocationRate  float64 `json:"allocation_rate"`
	UtilizationRate float64 `json:"utilization_rate"`
	LoadScore       float64 `json:"load_score"` // Weighted score
}

// LoadBalanceStats - Load balance statistics
type LoadBalanceStats struct {
	AvgAllocationRate float64 `json:"avg_allocation_rate"`
	StdDevAllocation  float64 `json:"stddev_allocation"`
	MaxAllocation     float64 `json:"max_allocation"`
	MinAllocation     float64 `json:"min_allocation"`
	Variance          float64 `json:"variance"`
}

// getFragmentationAnalysis - GET /api/v1/nodes/fragmentation-analysis
func getFragmentationAnalysis(c *gin.Context) {
	var params FragmentationAnalysisParams
	if err := c.ShouldBindQuery(&params); err != nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithError(err))
		return
	}

	// Get cluster clients
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(params.Cluster)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Get all nodes from database
	nodeFacade := database.GetFacadeForCluster(clients.ClusterName).GetNode()
	nodes, err := nodeFacade.ListGpuNodes(c.Request.Context())
	if err != nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.InternalError).WithError(err))
		return
	}

	if len(nodes) == 0 {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestDataNotExisted).WithMessage("No GPU nodes found"))
		return
	}

	// Get pods for allocation analysis
	podFacade := database.GetFacadeForCluster(clients.ClusterName).GetPod()

	// Calculate fragmentation for each node
	nodeFrags := make([]NodeFragmentation, 0, len(nodes))
	var totalScore float64

	for _, node := range nodes {
		frag := calculateNodeFragmentation(c.Request.Context(), node, podFacade)
		nodeFrags = append(nodeFrags, frag)
		totalScore += frag.FragmentationScore
	}

	// Calculate cluster-wide fragmentation score
	clusterScore := totalScore / float64(len(nodes))

	// Generate recommendations
	recommendations := generateFragmentationRecommendations(nodeFrags)

	// Build summary
	summary := buildFragmentationSummary(nodeFrags)

	response := FragmentationAnalysisResponse{
		Cluster:                   params.Cluster,
		ClusterFragmentationScore: clusterScore,
		TotalNodes:                len(nodes),
		NodeFragmentations:        nodeFrags,
		Recommendations:           recommendations,
		Summary:                   summary,
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// calculateNodeFragmentation - Calculate fragmentation score for a node
func calculateNodeFragmentation(ctx context.Context, node *dbModel.Node, podFacade database.PodFacadeInterface) NodeFragmentation {
	totalGPUs := node.GpuCount
	allocatedGPUs := node.GpuAllocation
	availableGPUs := totalGPUs - allocatedGPUs

	// Calculate fragmentation score
	// Factor 1: Basic allocation rate (inverted - high allocation = low fragmentation)
	allocationRate := float64(allocatedGPUs) / float64(totalGPUs)

	// Factor 2: Underutilization penalty
	// If allocated but not utilized, it's wasted resource
	utilizationGap := 0.0
	if node.GpuUtilization < allocationRate*100 {
		utilizationGap = (allocationRate*100 - node.GpuUtilization) / 100
	}

	// Factor 3: Partial allocation penalty
	// Get pods on this node to check allocation patterns
	pods, _ := podFacade.GetActiveGpuPodByNodeName(ctx, node.Name)
	partialAllocPenalty := calculatePartialAllocationPenalty(pods, totalGPUs)

	// Combined fragmentation score (0 = no fragmentation, 100 = severe fragmentation)
	// Higher score = more fragmented = worse
	baseFragmentation := (1 - allocationRate) * 40   // Up to 40 points for unused resources
	utilizationFragmentation := utilizationGap * 40  // Up to 40 points for allocated but unused
	partialFragmentation := partialAllocPenalty * 20 // Up to 20 points for suboptimal allocation

	score := baseFragmentation + utilizationFragmentation + partialFragmentation
	score = math.Min(score, 100)

	// Determine status
	status := determineFragmentationStatus(score)

	return NodeFragmentation{
		NodeName:           node.Name,
		TotalGPUs:          totalGPUs,
		AllocatedGPUs:      allocatedGPUs,
		AvailableGPUs:      availableGPUs,
		FragmentationScore: score,
		Status:             status,
		Utilization:        node.GpuUtilization,
	}
}

// calculatePartialAllocationPenalty - Calculate penalty for partial/suboptimal allocations
func calculatePartialAllocationPenalty(pods []*dbModel.GpuPods, totalGPUs int32) float64 {
	if len(pods) == 0 {
		return 0.0
	}

	// Count pods with different allocation sizes
	var smallAllocations int  // 1 GPU
	var mediumAllocations int // 2-3 GPUs
	var largeAllocations int  // 4+ GPUs

	for _, pod := range pods {
		if pod.GpuAllocated == 1 {
			smallAllocations++
		} else if pod.GpuAllocated <= 3 {
			mediumAllocations++
		} else {
			largeAllocations++
		}
	}

	// More small allocations = higher fragmentation risk
	// (harder to schedule large jobs)
	penalty := 0.0

	if totalGPUs >= 8 && smallAllocations > 2 {
		// On large nodes, too many small allocations is problematic
		penalty += 0.3
	}

	if smallAllocations > 4 {
		penalty += 0.4
	}

	return math.Min(penalty, 1.0)
}

// determineFragmentationStatus - Determine status based on score
func determineFragmentationStatus(score float64) string {
	if score < 30 {
		return "healthy"
	} else if score < 60 {
		return "fragmented"
	} else {
		return "critical"
	}
}

// buildFragmentationSummary - Build summary statistics
func buildFragmentationSummary(nodeFrags []NodeFragmentation) FragmentationSummary {
	var healthyNodes, fragmentedNodes, criticalNodes int
	var totalGPUs, totalAvailable int32

	for _, frag := range nodeFrags {
		switch frag.Status {
		case "healthy":
			healthyNodes++
		case "fragmented":
			fragmentedNodes++
		case "critical":
			criticalNodes++
		}
		totalGPUs += frag.TotalGPUs
		totalAvailable += frag.AvailableGPUs
	}

	// Estimate wasted GPUs (available but fragmented)
	wastedGPUs := 0
	for _, frag := range nodeFrags {
		if frag.Status != "healthy" && frag.AvailableGPUs > 0 {
			wastedGPUs += int(frag.AvailableGPUs)
		}
	}

	wastePercentage := 0.0
	if totalGPUs > 0 {
		wastePercentage = float64(wastedGPUs) / float64(totalGPUs) * 100
	}

	return FragmentationSummary{
		HealthyNodes:    healthyNodes,
		FragmentedNodes: fragmentedNodes,
		CriticalNodes:   criticalNodes,
		TotalWastedGPUs: wastedGPUs,
		WastePercentage: wastePercentage,
	}
}

// generateFragmentationRecommendations - Generate recommendations based on analysis
func generateFragmentationRecommendations(nodeFrags []NodeFragmentation) []string {
	recommendations := make([]string, 0)

	// Check for critical nodes
	criticalCount := 0
	for _, frag := range nodeFrags {
		if frag.Status == "critical" {
			criticalCount++
		}
	}

	if criticalCount > 0 {
		recommendations = append(recommendations,
			"Critical fragmentation detected on some nodes. Consider pod consolidation or rebalancing.")
	}

	// Check for underutilization
	var lowUtilNodes []string
	for _, frag := range nodeFrags {
		if frag.AllocatedGPUs > 0 && frag.Utilization < 30 {
			lowUtilNodes = append(lowUtilNodes, frag.NodeName)
		}
	}

	if len(lowUtilNodes) > 0 {
		recommendations = append(recommendations,
			"Some nodes have allocated GPUs with low utilization. Check if pods are idle.")
	}

	// Check for inefficient allocation patterns
	var fragmentedNodes []string
	for _, frag := range nodeFrags {
		if frag.Status == "fragmented" && frag.AvailableGPUs > 0 {
			fragmentedNodes = append(fragmentedNodes, frag.NodeName)
		}
	}

	if len(fragmentedNodes) > 0 {
		recommendations = append(recommendations,
			"Some nodes have fragmented GPU allocation. Consider using pod affinity/anti-affinity rules.")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Cluster GPU allocation is healthy. No immediate action needed.")
	}

	return recommendations
}

// getNodeFragmentation - GET /api/v1/nodes/:name/fragmentation
func getNodeFragmentation(c *gin.Context) {
	nodeName := c.Param("name")
	if nodeName == "" {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithMessage("node_name is required"))
		return
	}

	cluster := c.Query("cluster")
	if cluster == "" {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithMessage("cluster is required"))
		return
	}

	// Get cluster clients
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(cluster)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Get node information
	nodeFacade := database.GetFacadeForCluster(clients.ClusterName).GetNode()
	node, err := nodeFacade.GetNodeByName(c.Request.Context(), nodeName)
	if err != nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.InternalError).WithError(err))
		return
	}
	if node == nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestDataNotExisted).WithMessage("Node not found"))
		return
	}

	// Get pods on this node
	podFacade := database.GetFacadeForCluster(clients.ClusterName).GetPod()
	pods, err := podFacade.GetActiveGpuPodByNodeName(c.Request.Context(), nodeName)
	if err != nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.InternalError).WithError(err))
		return
	}

	// Calculate fragmentation
	frag := calculateNodeFragmentation(c.Request.Context(), node, podFacade)

	// Build allocation pattern
	pattern := buildAllocationPattern(pods, node.GpuCount)

	// Build pod allocations list
	podAllocations := make([]PodAllocation, 0, len(pods))
	for _, pod := range pods {
		podAllocations = append(podAllocations, PodAllocation{
			PodName:       pod.Name,
			Namespace:     pod.Namespace,
			AllocatedGPUs: pod.GpuAllocated,
		})
	}

	// Generate node-specific recommendations
	nodeRecommendations := generateNodeRecommendations(frag, pattern)

	response := NodeFragmentationDetail{
		NodeName:           nodeName,
		TotalGPUs:          frag.TotalGPUs,
		AllocatedGPUs:      frag.AllocatedGPUs,
		AvailableGPUs:      frag.AvailableGPUs,
		FragmentationScore: frag.FragmentationScore,
		Status:             frag.Status,
		AllocationPattern:  pattern,
		RunningPods:        podAllocations,
		Recommendations:    nodeRecommendations,
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// buildAllocationPattern - Build allocation pattern information
func buildAllocationPattern(pods []*dbModel.GpuPods, totalGPUs int32) AllocationPattern {
	fullyAlloc := 0
	partialAlloc := 0

	for _, pod := range pods {
		if pod.GpuAllocated >= 4 {
			fullyAlloc++
		} else if pod.GpuAllocated > 0 {
			partialAlloc++
		}
	}

	// Estimate largest contiguous GPU block
	// Simplified: assume available GPUs form largest contiguous block
	allocatedGPUs := int32(0)
	for _, pod := range pods {
		allocatedGPUs += pod.GpuAllocated
	}
	largestContiguous := int(totalGPUs - allocatedGPUs)

	return AllocationPattern{
		FullyAllocatedPods:   fullyAlloc,
		PartiallyAllocPods:   partialAlloc,
		GPUSharing:           false, // TODO: Detect GPU sharing from pod annotations (currently simplified)
		LargestContiguousGPU: largestContiguous,
	}
}

// generateNodeRecommendations - Generate node-specific recommendations
func generateNodeRecommendations(frag NodeFragmentation, pattern AllocationPattern) []string {
	recommendations := make([]string, 0)

	if frag.Status == "critical" {
		recommendations = append(recommendations,
			"Critical fragmentation: Consider migrating some pods to other nodes")
	}

	if pattern.PartiallyAllocPods > 3 && frag.TotalGPUs >= 8 {
		recommendations = append(recommendations,
			"Many small GPU allocations detected. Consider consolidating workloads")
	}

	if frag.AvailableGPUs > 0 && frag.AvailableGPUs < 4 && pattern.LargestContiguousGPU < 4 {
		recommendations = append(recommendations,
			"Limited contiguous GPU blocks. Difficult to schedule larger jobs")
	}

	if frag.Utilization < 30 && frag.AllocatedGPUs > 0 {
		recommendations = append(recommendations,
			"Low GPU utilization despite allocation. Check if pods are idle or waiting")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Node GPU allocation is healthy")
	}

	return recommendations
}

// getLoadBalanceAnalysis - GET /api/v1/nodes/load-balance-analysis
func getLoadBalanceAnalysis(c *gin.Context) {
	var params FragmentationAnalysisParams
	if err := c.ShouldBindQuery(&params); err != nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithError(err))
		return
	}

	// Get cluster clients
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(params.Cluster)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Get all nodes
	nodeFacade := database.GetFacadeForCluster(clients.ClusterName).GetNode()
	nodes, err := nodeFacade.ListGpuNodes(c.Request.Context())
	if err != nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.InternalError).WithError(err))
		return
	}

	if len(nodes) == 0 {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestDataNotExisted).WithMessage("No GPU nodes found"))
		return
	}

	// Calculate load for each node
	nodeLoads := make([]NodeLoad, 0, len(nodes))
	for _, node := range nodes {
		load := calculateNodeLoad(node)
		nodeLoads = append(nodeLoads, load)
	}

	// Calculate load balance score
	loadBalanceScore := calculateLoadBalanceScore(nodeLoads)

	// Identify hotspot and idle nodes
	hotspotNodes, idleNodes := identifyHotspotAndIdleNodes(nodeLoads)

	// Calculate statistics
	stats := calculateLoadBalanceStats(nodeLoads)

	// Generate recommendations
	recommendations := generateLoadBalanceRecommendations(nodeLoads, hotspotNodes, idleNodes)

	response := LoadBalanceAnalysisResponse{
		Cluster:              params.Cluster,
		LoadBalanceScore:     loadBalanceScore,
		NodeLoadDistribution: nodeLoads,
		HotspotNodes:         hotspotNodes,
		IdleNodes:            idleNodes,
		Recommendations:      recommendations,
		Statistics:           stats,
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// calculateNodeLoad - Calculate load for a single node
func calculateNodeLoad(node *dbModel.Node) NodeLoad {
	allocationRate := 0.0
	if node.GpuCount > 0 {
		allocationRate = float64(node.GpuAllocation) / float64(node.GpuCount) * 100
	}

	utilizationRate := node.GpuUtilization

	// Weighted load score (allocation 60%, utilization 40%)
	loadScore := allocationRate*0.6 + utilizationRate*0.4

	return NodeLoad{
		NodeName:        node.Name,
		AllocationRate:  allocationRate,
		UtilizationRate: utilizationRate,
		LoadScore:       loadScore,
	}
}

// calculateLoadBalanceScore - Calculate overall load balance score
// Higher score = better balanced (0-100)
func calculateLoadBalanceScore(nodeLoads []NodeLoad) float64 {
	if len(nodeLoads) == 0 {
		return 100
	}

	// Calculate mean allocation rate
	var sum float64
	for _, load := range nodeLoads {
		sum += load.AllocationRate
	}
	mean := sum / float64(len(nodeLoads))

	// Calculate standard deviation
	var variance float64
	for _, load := range nodeLoads {
		variance += math.Pow(load.AllocationRate-mean, 2)
	}
	variance /= float64(len(nodeLoads))
	stddev := math.Sqrt(variance)

	// Coefficient of Variation
	cv := 0.0
	if mean > 0 {
		cv = stddev / mean
	}

	// Convert to score (lower CV = higher score)
	// CV of 0 = perfect balance = 100 score
	// CV of 1 or more = poor balance = 0 score
	score := 100 * (1 - math.Min(cv, 1))

	return score
}

// identifyHotspotAndIdleNodes - Identify hotspot and idle nodes
func identifyHotspotAndIdleNodes(nodeLoads []NodeLoad) ([]string, []string) {
	hotspotNodes := make([]string, 0)
	idleNodes := make([]string, 0)

	// Calculate mean load
	var sum float64
	for _, load := range nodeLoads {
		sum += load.LoadScore
	}
	mean := sum / float64(len(nodeLoads))

	// Nodes with load > mean + 20 are hotspots
	// Nodes with load < mean - 20 are idle
	for _, load := range nodeLoads {
		if load.LoadScore > mean+20 {
			hotspotNodes = append(hotspotNodes, load.NodeName)
		} else if load.LoadScore < mean-20 {
			idleNodes = append(idleNodes, load.NodeName)
		}
	}

	return hotspotNodes, idleNodes
}

// calculateLoadBalanceStats - Calculate load balance statistics
func calculateLoadBalanceStats(nodeLoads []NodeLoad) LoadBalanceStats {
	if len(nodeLoads) == 0 {
		return LoadBalanceStats{}
	}

	// Calculate statistics
	var sum float64
	var max, min float64 = 0, 100

	for _, load := range nodeLoads {
		sum += load.AllocationRate
		if load.AllocationRate > max {
			max = load.AllocationRate
		}
		if load.AllocationRate < min {
			min = load.AllocationRate
		}
	}

	mean := sum / float64(len(nodeLoads))

	// Calculate variance and stddev
	var variance float64
	for _, load := range nodeLoads {
		variance += math.Pow(load.AllocationRate-mean, 2)
	}
	variance /= float64(len(nodeLoads))
	stddev := math.Sqrt(variance)

	return LoadBalanceStats{
		AvgAllocationRate: mean,
		StdDevAllocation:  stddev,
		MaxAllocation:     max,
		MinAllocation:     min,
		Variance:          variance,
	}
}

// generateLoadBalanceRecommendations - Generate load balance recommendations
func generateLoadBalanceRecommendations(nodeLoads []NodeLoad, hotspotNodes, idleNodes []string) []string {
	recommendations := make([]string, 0)

	if len(hotspotNodes) > 0 {
		recommendations = append(recommendations,
			"Hotspot nodes detected with high GPU load. Consider rebalancing workloads.")
	}

	if len(idleNodes) > 0 {
		recommendations = append(recommendations,
			"Idle nodes detected with low GPU utilization. Consider consolidating workloads or draining nodes.")
	}

	// Check for high variance
	stats := calculateLoadBalanceStats(nodeLoads)
	if stats.StdDevAllocation > 20 {
		recommendations = append(recommendations,
			"High variance in node allocation. Consider implementing pod scheduling strategies.")
	}

	// Check for overall low utilization
	if stats.AvgAllocationRate < 40 {
		recommendations = append(recommendations,
			"Overall cluster GPU allocation is low. Consider optimizing resource requests.")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations,
			"Cluster load is well balanced. No immediate action needed.")
	}

	return recommendations
}
