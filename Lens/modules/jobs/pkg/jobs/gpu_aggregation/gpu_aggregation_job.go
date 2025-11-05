package gpu_aggregation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	"github.com/AMD-AGI/primus-lens/core/pkg/database/filter"
	dbmodel "github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/config"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/prom"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/model"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/common"
)

const (
	// ConfigKeyGpuAggregation GPU聚合任务的配置键
	ConfigKeyGpuAggregation = "job.gpu_aggregation.config"
)

// GpuAggregationJob GPU使用率聚合Job
type GpuAggregationJob struct {
	config        *model.GpuAggregationConfig
	snapshotCache []GpuSnapshot   // 内存缓存当前小时的快照
	currentHour   time.Time       // 当前正在采样的小时
	configManager *config.Manager // 配置管理器
	clusterName   string          // 集群名称
}

// GpuSnapshot GPU采样快照
type GpuSnapshot struct {
	Timestamp      time.Time
	ClusterName    string
	TotalCapacity  int
	AllocatedGPU   int
	UtilizationSum float64
	ActiveGPUCount int

	// 维度数据
	NamespaceData  map[string]*NamespaceGpuData
	LabelData      map[string]map[string]*LabelGpuData // labelKey -> labelValue -> data
	AnnotationData map[string]map[string]*LabelGpuData // annotationKey -> annotationValue -> data
}

// NamespaceGpuData Namespace维度的GPU数据
type NamespaceGpuData struct {
	Namespace      string
	AllocatedGPU   int
	UtilizationSum float64
	WorkloadCount  int
	Workloads      []model.WorkloadSnapshot
}

// LabelGpuData Label/Annotation维度的GPU数据
type LabelGpuData struct {
	DimensionType  string // 'label' 或 'annotation'
	DimensionKey   string
	DimensionValue string
	AllocatedGPU   int
	UtilizationSum float64
	WorkloadCount  int
}

// NewGpuAggregationJob 创建新的聚合Job
// clusterName: 集群名称，为空则使用当前集群
func NewGpuAggregationJob() *GpuAggregationJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()

	return &GpuAggregationJob{
		config:        nil,                        // 配置将在 Run 时从配置管理器读取
		snapshotCache: make([]GpuSnapshot, 0, 12), // 每小时12个快照(5分钟间隔)
		currentHour:   time.Now().Truncate(time.Hour),
		configManager: config.GetConfigManagerForCluster(clusterName),
		clusterName:   clusterName,
	}
}

// NewGpuAggregationJobWithConfig 使用指定配置创建聚合Job（保留兼容性）
func NewGpuAggregationJobWithConfig(cfg *model.GpuAggregationConfig) *GpuAggregationJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	return &GpuAggregationJob{
		config:        cfg,
		snapshotCache: make([]GpuSnapshot, 0, 12), // 每小时12个快照(5分钟间隔)
		currentHour:   time.Now().Truncate(time.Hour),
		configManager: config.GetConfigManagerForCluster(clusterName),
		clusterName:   clusterName,
	}
}

// Run 运行Job (由Job调度器调用)
func (j *GpuAggregationJob) Run(ctx context.Context,
	k8sClientSet *clientsets.K8SClientSet,
	storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {

	stats := common.NewExecutionStats()

	// 如果配置为 nil，从配置管理器读取
	if j.config == nil {
		if err := j.loadConfig(ctx); err != nil {
			log.Warnf("Failed to load GPU aggregation config, job will not run: %v", err)
			stats.AddMessage("GPU aggregation config not found, job disabled")
			return stats, nil // 返回 nil 不影响调度器继续运行
		}
	}

	// 检查配置是否启用
	if !j.config.Enabled {
		log.Debugf("GPU aggregation job is disabled in config")
		stats.AddMessage("GPU aggregation job is disabled in config")
		return stats, nil
	}

	clusterName := j.clusterName
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// 检查是否需要执行小时聚合
	now := time.Now()
	currentHour := now.Truncate(time.Hour)

	// 如果跨小时了,先聚合上一个小时的数据
	if currentHour.After(j.currentHour) && len(j.snapshotCache) > 0 {
		log.Infof("Hour changed, aggregating data for hour: %v", j.currentHour)
		aggStart := time.Now()
		if err := j.aggregateHourlyData(ctx, clusterName, j.currentHour); err != nil {
			stats.ErrorCount++
			log.Errorf("Failed to aggregate hourly data: %v", err)
			// 不返回错误,继续采样
		} else {
			stats.ProcessDuration += time.Since(aggStart).Seconds()
			stats.ItemsCreated++ // 创建了一个小时聚合记录
			stats.AddMessage(fmt.Sprintf("Aggregated hourly data for %v", j.currentHour))
		}

		// 清空缓存,开始新的一小时
		j.snapshotCache = j.snapshotCache[:0]
		j.currentHour = currentHour
	}

	// 执行采样
	if j.config.Sampling.Enabled {
		sampleStart := time.Now()
		if err := j.sample(ctx, clusterName, k8sClientSet, storageClientSet); err != nil {
			log.Errorf("Failed to sample GPU data: %v", err)
			return stats, err
		}
		stats.QueryDuration = time.Since(sampleStart).Seconds()
		stats.RecordsProcessed = int64(len(j.snapshotCache))
		stats.AddCustomMetric("snapshots_cached", len(j.snapshotCache))
		stats.AddMessage("GPU data sampled successfully")
	}

	return stats, nil
}

// sample 采样当前GPU状态
func (j *GpuAggregationJob) sample(ctx context.Context,
	clusterName string,
	k8sClientSet *clientsets.K8SClientSet,
	storageClientSet *clientsets.StorageClientSet) error {

	startTime := time.Now()
	log.Infof("Starting GPU usage sampling for cluster: %s", clusterName)

	snapshot := GpuSnapshot{
		Timestamp:      startTime,
		ClusterName:    clusterName,
		NamespaceData:  make(map[string]*NamespaceGpuData),
		LabelData:      make(map[string]map[string]*LabelGpuData),
		AnnotationData: make(map[string]map[string]*LabelGpuData),
	}

	// 1. 从数据库获取集群GPU总容量
	// 通过数据库查询所有GPU节点的总容量
	totalCapacity, err := j.getClusterGpuCapacity(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("failed to get GPU capacity: %w", err)
	}
	snapshot.TotalCapacity = totalCapacity

	// 2. 从数据库获取所有活跃的GPU Pods
	dbPods, err := database.GetFacadeForCluster(clusterName).GetPod().ListActiveGpuPods(ctx)
	if err != nil {
		return fmt.Errorf("failed to list active GPU pods from database: %w", err)
	}

	log.Infof("Found %d active GPU pods in database", len(dbPods))

	// 3. 处理每个Pod,收集GPU分配和使用率
	allocatedGPU := 0
	utilizationSum := 0.0
	activeGPUCount := 0

	// 构建 PodUID -> Workload 的映射关系
	podUIDToWorkload, err := j.buildPodToWorkloadMapping(ctx, clusterName, dbPods)
	if err != nil {
		log.Warnf("Failed to build pod to workload mapping: %v", err)
		// 继续处理，即使没有 workload 信息
	}

	for _, dbPod := range dbPods {
		gpuRequest := int(dbPod.GpuAllocated)
		if gpuRequest == 0 {
			continue
		}

		allocatedGPU += gpuRequest

		// 从Prometheus查询该Pod的GPU使用率
		utilization, err := j.queryWorkloadUtilization(ctx, storageClientSet, dbPod.UID)
		if err != nil {
			log.Warnf("Failed to query utilization for pod %s: %v", dbPod.UID, err)
			utilization = 0 // 查询失败时使用0
		}

		utilizationSum += utilization * float64(gpuRequest)
		activeGPUCount += gpuRequest

		// 获取该Pod关联的Workload信息（用于获取labels和annotations）
		workload := podUIDToWorkload[dbPod.UID]

		// 4. 收集namespace维度数据
		j.collectNamespaceDataFromDB(&snapshot, dbPod, workload, gpuRequest, utilization)

		// 5. 收集label维度数据
		j.collectLabelDataFromDB(&snapshot, dbPod, workload, gpuRequest, utilization)

		// 6. 收集annotation维度数据
		j.collectAnnotationDataFromDB(&snapshot, dbPod, workload, gpuRequest, utilization)
	}

	snapshot.AllocatedGPU = allocatedGPU
	snapshot.UtilizationSum = utilizationSum
	snapshot.ActiveGPUCount = activeGPUCount

	// 6. 保存快照到缓存
	j.snapshotCache = append(j.snapshotCache, snapshot)

	// 7. 保存快照到数据库(可选,用于调试和审计)
	if err := j.saveSnapshotToDatabase(ctx, &snapshot); err != nil {
		log.Warnf("Failed to save snapshot to database: %v", err)
		// 不返回错误,快照保存失败不影响采样流程
	}

	duration := time.Since(startTime)
	log.Infof("GPU usage sampling completed for cluster: %s, took: %v, allocated: %d/%d GPUs",
		clusterName, duration, allocatedGPU, totalCapacity)

	// TODO: 导出Prometheus指标
	// j.exportMetrics(&snapshot)

	return nil
}

// collectNamespaceDataFromDB 从数据库Pod收集namespace维度的数据
func (j *GpuAggregationJob) collectNamespaceDataFromDB(
	snapshot *GpuSnapshot,
	dbPod *dbmodel.GpuPods,
	workload *dbmodel.GpuWorkload,
	gpuRequest int,
	utilization float64) {

	namespace := dbPod.Namespace

	// 检查是否需要排除该namespace
	if j.shouldExcludeNamespace(namespace) {
		return
	}

	nsData, exists := snapshot.NamespaceData[namespace]
	if !exists {
		nsData = &NamespaceGpuData{
			Namespace: namespace,
			Workloads: make([]model.WorkloadSnapshot, 0),
		}
		snapshot.NamespaceData[namespace] = nsData
	}

	nsData.AllocatedGPU += gpuRequest
	nsData.UtilizationSum += utilization * float64(gpuRequest)
	nsData.WorkloadCount++

	// 记录workload信息
	workloadKind := ""
	if workload != nil {
		workloadKind = workload.Kind
	}

	workloadSnapshot := model.WorkloadSnapshot{
		UID:          dbPod.UID,
		Name:         dbPod.Name,
		Namespace:    dbPod.Namespace,
		Kind:         workloadKind,
		AllocatedGPU: gpuRequest,
		Utilization:  utilization,
	}
	nsData.Workloads = append(nsData.Workloads, workloadSnapshot)
}

// collectLabelDataFromDB 从数据库Workload收集label维度的数据
func (j *GpuAggregationJob) collectLabelDataFromDB(
	snapshot *GpuSnapshot,
	dbPod *dbmodel.GpuPods,
	workload *dbmodel.GpuWorkload,
	gpuRequest int,
	utilization float64) {

	if !j.config.Dimensions.Label.Enabled {
		return
	}

	if workload == nil {
		// 如果没有workload信息，无法获取labels
		return
	}

	// 从workload的Labels字段（ExtType/map[string]interface{}）获取labels
	labels := make(map[string]string)
	if workload.Labels != nil {
		for k, v := range workload.Labels {
			if strVal, ok := v.(string); ok {
				labels[k] = strVal
			}
		}
	}

	// 遍历配置的label keys
	for _, labelKey := range j.config.Dimensions.Label.LabelKeys {
		labelValue := labels[labelKey]
		if labelValue == "" {
			labelValue = j.config.Dimensions.Label.DefaultValue
		}

		// 确保labelKey的map存在
		if _, exists := snapshot.LabelData[labelKey]; !exists {
			snapshot.LabelData[labelKey] = make(map[string]*LabelGpuData)
		}

		labelData, exists := snapshot.LabelData[labelKey][labelValue]
		if !exists {
			labelData = &LabelGpuData{
				DimensionType:  "label",
				DimensionKey:   labelKey,
				DimensionValue: labelValue,
			}
			snapshot.LabelData[labelKey][labelValue] = labelData
		}

		labelData.AllocatedGPU += gpuRequest
		labelData.UtilizationSum += utilization * float64(gpuRequest)
		labelData.WorkloadCount++
	}
}

// collectAnnotationDataFromDB 从数据库收集annotation维度的数据
func (j *GpuAggregationJob) collectAnnotationDataFromDB(
	snapshot *GpuSnapshot,
	dbPod *dbmodel.GpuPods,
	workload *dbmodel.GpuWorkload,
	gpuRequest int,
	utilization float64) {

	if !j.config.Dimensions.Label.Enabled {
		return
	}

	if workload == nil {
		// 如果没有workload信息，无法获取annotations
		return
	}

	// 从workload的Annotations字段（ExtType/map[string]interface{}）获取annotations
	annotations := make(map[string]string)
	if workload.Annotations != nil {
		for k, v := range workload.Annotations {
			if strVal, ok := v.(string); ok {
				annotations[k] = strVal
			}
		}
	}

	// 遍历配置的annotation keys
	for _, annotationKey := range j.config.Dimensions.Label.AnnotationKeys {
		annotationValue := annotations[annotationKey]
		if annotationValue == "" {
			annotationValue = j.config.Dimensions.Label.DefaultValue
		}

		// 确保annotationKey的map存在
		if _, exists := snapshot.AnnotationData[annotationKey]; !exists {
			snapshot.AnnotationData[annotationKey] = make(map[string]*LabelGpuData)
		}

		annData, exists := snapshot.AnnotationData[annotationKey][annotationValue]
		if !exists {
			annData = &LabelGpuData{
				DimensionType:  "annotation",
				DimensionKey:   annotationKey,
				DimensionValue: annotationValue,
			}
			snapshot.AnnotationData[annotationKey][annotationValue] = annData
		}

		annData.AllocatedGPU += gpuRequest
		annData.UtilizationSum += utilization * float64(gpuRequest)
		annData.WorkloadCount++
	}
}

// shouldExcludeNamespace 判断是否应该排除该namespace
func (j *GpuAggregationJob) shouldExcludeNamespace(namespace string) bool {
	if !j.config.Dimensions.Namespace.Enabled {
		return true
	}

	// 检查是否在排除列表中
	for _, excluded := range j.config.Dimensions.Namespace.ExcludeNamespaces {
		if namespace == excluded {
			return true
		}
	}

	// 检查是否为系统namespace
	if !j.config.Dimensions.Namespace.IncludeSystemNamespaces {
		systemNamespaces := []string{"kube-system", "kube-public", "kube-node-lease"}
		for _, sysNs := range systemNamespaces {
			if namespace == sysNs {
				return true
			}
		}
	}

	return false
}

// queryWorkloadUtilization 查询workload的GPU使用率
func (j *GpuAggregationJob) queryWorkloadUtilization(
	ctx context.Context,
	storageClientSet *clientsets.StorageClientSet,
	workloadUID string) (float64, error) {

	// 使用配置的查询模板
	query := fmt.Sprintf(j.config.Prometheus.WorkloadUtilizationQuery, workloadUID)

	// 查询最近1分钟的平均值
	endTime := time.Now()
	startTime := endTime.Add(-1 * time.Minute)

	series, err := prom.QueryRange(ctx, storageClientSet, query, startTime, endTime,
		j.config.Prometheus.QueryStep, map[string]struct{}{"__name__": {}})

	if err != nil {
		return 0, err
	}

	if len(series) == 0 || len(series[0].Values) == 0 {
		return 0, nil
	}

	// 计算平均值
	sum := 0.0
	for _, point := range series[0].Values {
		sum += point.Value
	}
	avg := sum / float64(len(series[0].Values))

	return avg, nil
}

// aggregateHourlyData 聚合小时数据
func (j *GpuAggregationJob) aggregateHourlyData(
	ctx context.Context,
	clusterName string,
	hour time.Time) error {

	if len(j.snapshotCache) == 0 {
		log.Warnf("No snapshots to aggregate for hour: %v", hour)
		return nil
	}

	log.Infof("Aggregating %d snapshots for hour: %v", len(j.snapshotCache), hour)
	startTime := time.Now()

	// 1. 聚合集群级别数据
	if j.config.Dimensions.Cluster.Enabled {
		clusterStats := j.aggregateClusterStats(clusterName, hour)
		if err := j.saveClusterStats(ctx, clusterStats); err != nil {
			log.Errorf("Failed to save cluster stats: %v", err)
			return err
		}
	}

	// 2. 聚合namespace级别数据
	if j.config.Dimensions.Namespace.Enabled {
		namespaceStats := j.aggregateNamespaceStats(clusterName, hour)
		if err := j.saveNamespaceStats(ctx, namespaceStats); err != nil {
			log.Errorf("Failed to save namespace stats: %v", err)
			return err
		}
	}

	// 3. 聚合label/annotation级别数据
	if j.config.Dimensions.Label.Enabled {
		labelStats := j.aggregateLabelStats(clusterName, hour)
		if err := j.saveLabelStats(ctx, labelStats); err != nil {
			log.Errorf("Failed to save label/annotation stats: %v", err)
			return err
		}
	}

	duration := time.Since(startTime)
	log.Infof("Hourly aggregation completed for hour: %v, took: %v", hour, duration)

	return nil
}

// aggregateClusterStats 聚合集群级别统计
func (j *GpuAggregationJob) aggregateClusterStats(
	clusterName string,
	hour time.Time) *model.ClusterGpuHourlyStats {

	stats := &model.ClusterGpuHourlyStats{
		ClusterName: clusterName,
		StatHour:    hour,
		SampleCount: len(j.snapshotCache),
	}

	totalCapacitySum := 0
	allocatedSum := 0
	utilizationValues := make([]float64, 0, len(j.snapshotCache))

	for _, snapshot := range j.snapshotCache {
		totalCapacitySum += snapshot.TotalCapacity
		allocatedSum += snapshot.AllocatedGPU

		// 计算该快照的平均使用率
		var utilization float64
		if snapshot.ActiveGPUCount > 0 {
			utilization = snapshot.UtilizationSum / float64(snapshot.ActiveGPUCount)
		}
		utilizationValues = append(utilizationValues, utilization)
	}

	// 计算平均值
	count := float64(len(j.snapshotCache))
	stats.TotalGpuCapacity = int(float64(totalCapacitySum) / count)
	stats.AllocatedGpuCount = float64(allocatedSum) / count

	if stats.TotalGpuCapacity > 0 {
		stats.AllocationRate = (stats.AllocatedGpuCount / float64(stats.TotalGpuCapacity)) * 100
	}

	// 计算使用率统计
	sort.Float64s(utilizationValues)
	stats.MinUtilization = utilizationValues[0]
	stats.MaxUtilization = utilizationValues[len(utilizationValues)-1]
	stats.P50Utilization = calculatePercentile(utilizationValues, 0.50)
	stats.P95Utilization = calculatePercentile(utilizationValues, 0.95)

	utilizationSum := 0.0
	for _, v := range utilizationValues {
		utilizationSum += v
	}
	stats.AvgUtilization = utilizationSum / count

	return stats
}

// aggregateNamespaceStats 聚合namespace级别统计
func (j *GpuAggregationJob) aggregateNamespaceStats(
	clusterName string,
	hour time.Time) []*model.NamespaceGpuHourlyStats {

	// 先聚合所有快照中相同namespace的数据
	namespaceAggregates := make(map[string]*namespaceAggregate)

	for _, snapshot := range j.snapshotCache {
		for namespace, data := range snapshot.NamespaceData {
			agg, exists := namespaceAggregates[namespace]
			if !exists {
				agg = &namespaceAggregate{
					namespace:         namespace,
					allocatedSum:      0,
					utilizationValues: make([]float64, 0),
					workloadCountSum:  0,
				}
				namespaceAggregates[namespace] = agg
			}

			agg.allocatedSum += data.AllocatedGPU
			agg.workloadCountSum += data.WorkloadCount

			// 计算该快照中该namespace的平均使用率
			var nsUtilization float64
			if data.AllocatedGPU > 0 {
				nsUtilization = data.UtilizationSum / float64(data.AllocatedGPU)
			}
			agg.utilizationValues = append(agg.utilizationValues, nsUtilization)
		}
	}

	// 转换为数据库模型
	results := make([]*model.NamespaceGpuHourlyStats, 0, len(namespaceAggregates))
	count := float64(len(j.snapshotCache))

	for namespace, agg := range namespaceAggregates {
		stats := &model.NamespaceGpuHourlyStats{
			ClusterName:         clusterName,
			Namespace:           namespace,
			StatHour:            hour,
			AllocatedGpuCount:   float64(agg.allocatedSum) / count,
			ActiveWorkloadCount: int(float64(agg.workloadCountSum) / count),
		}

		// 计算使用率统计
		sort.Float64s(agg.utilizationValues)
		if len(agg.utilizationValues) > 0 {
			stats.MinUtilization = agg.utilizationValues[0]
			stats.MaxUtilization = agg.utilizationValues[len(agg.utilizationValues)-1]

			utilizationSum := 0.0
			for _, v := range agg.utilizationValues {
				utilizationSum += v
			}
			stats.AvgUtilization = utilizationSum / float64(len(agg.utilizationValues))
		}

		results = append(results, stats)
	}

	return results
}

// aggregateLabelStats 聚合label和annotation级别统计
func (j *GpuAggregationJob) aggregateLabelStats(
	clusterName string,
	hour time.Time) []*model.LabelGpuHourlyStats {

	// dimensionType -> dimensionKey -> dimensionValue -> aggregate
	labelAggregates := make(map[string]map[string]map[string]*labelAggregate)

	// 聚合label数据
	for _, snapshot := range j.snapshotCache {
		for labelKey, valueMap := range snapshot.LabelData {
			if _, exists := labelAggregates["label"]; !exists {
				labelAggregates["label"] = make(map[string]map[string]*labelAggregate)
			}
			if _, exists := labelAggregates["label"][labelKey]; !exists {
				labelAggregates["label"][labelKey] = make(map[string]*labelAggregate)
			}

			for labelValue, data := range valueMap {
				agg, exists := labelAggregates["label"][labelKey][labelValue]
				if !exists {
					agg = &labelAggregate{
						dimensionType:     "label",
						dimensionKey:      labelKey,
						dimensionValue:    labelValue,
						allocatedSum:      0,
						utilizationValues: make([]float64, 0),
						workloadCountSum:  0,
					}
					labelAggregates["label"][labelKey][labelValue] = agg
				}

				agg.allocatedSum += data.AllocatedGPU
				agg.workloadCountSum += data.WorkloadCount

				var utilization float64
				if data.AllocatedGPU > 0 {
					utilization = data.UtilizationSum / float64(data.AllocatedGPU)
				}
				agg.utilizationValues = append(agg.utilizationValues, utilization)
			}
		}

		// 聚合annotation数据
		for annotationKey, valueMap := range snapshot.AnnotationData {
			if _, exists := labelAggregates["annotation"]; !exists {
				labelAggregates["annotation"] = make(map[string]map[string]*labelAggregate)
			}
			if _, exists := labelAggregates["annotation"][annotationKey]; !exists {
				labelAggregates["annotation"][annotationKey] = make(map[string]*labelAggregate)
			}

			for annotationValue, data := range valueMap {
				agg, exists := labelAggregates["annotation"][annotationKey][annotationValue]
				if !exists {
					agg = &labelAggregate{
						dimensionType:     "annotation",
						dimensionKey:      annotationKey,
						dimensionValue:    annotationValue,
						allocatedSum:      0,
						utilizationValues: make([]float64, 0),
						workloadCountSum:  0,
					}
					labelAggregates["annotation"][annotationKey][annotationValue] = agg
				}

				agg.allocatedSum += data.AllocatedGPU
				agg.workloadCountSum += data.WorkloadCount

				var utilization float64
				if data.AllocatedGPU > 0 {
					utilization = data.UtilizationSum / float64(data.AllocatedGPU)
				}
				agg.utilizationValues = append(agg.utilizationValues, utilization)
			}
		}
	}

	// 转换为数据库模型
	results := make([]*model.LabelGpuHourlyStats, 0)
	count := float64(len(j.snapshotCache))

	for dimensionType, keyMap := range labelAggregates {
		for dimensionKey, valueMap := range keyMap {
			for dimensionValue, agg := range valueMap {
				stats := &model.LabelGpuHourlyStats{
					ClusterName:         clusterName,
					DimensionType:       dimensionType,
					DimensionKey:        dimensionKey,
					DimensionValue:      dimensionValue,
					StatHour:            hour,
					AllocatedGpuCount:   float64(agg.allocatedSum) / count,
					ActiveWorkloadCount: int(float64(agg.workloadCountSum) / count),
				}

				// 计算使用率统计
				sort.Float64s(agg.utilizationValues)
				if len(agg.utilizationValues) > 0 {
					stats.MinUtilization = agg.utilizationValues[0]
					stats.MaxUtilization = agg.utilizationValues[len(agg.utilizationValues)-1]

					utilizationSum := 0.0
					for _, v := range agg.utilizationValues {
						utilizationSum += v
					}
					stats.AvgUtilization = utilizationSum / float64(len(agg.utilizationValues))
				}

				results = append(results, stats)
			}
		}
	}

	return results
}

// 辅助结构体
type namespaceAggregate struct {
	namespace         string
	allocatedSum      int
	utilizationValues []float64
	workloadCountSum  int
}

type labelAggregate struct {
	dimensionType     string
	dimensionKey      string
	dimensionValue    string
	allocatedSum      int
	utilizationValues []float64
	workloadCountSum  int
}

// saveClusterStats 保存集群级别统计到数据库
func (j *GpuAggregationJob) saveClusterStats(
	ctx context.Context,
	stats *model.ClusterGpuHourlyStats) error {

	// 转换为数据库模型
	dbStats := convertToDBClusterStats(stats)

	facade := database.GetFacade().GetGpuAggregation()
	if err := facade.SaveClusterHourlyStats(ctx, dbStats); err != nil {
		return fmt.Errorf("failed to save cluster stats: %w", err)
	}

	log.Infof("Cluster stats saved for %s at %v: allocation=%.2f%%, utilization=%.2f%%",
		stats.ClusterName, stats.StatHour, stats.AllocationRate, stats.AvgUtilization)

	return nil
}

// saveNamespaceStats 保存namespace级别统计到数据库
func (j *GpuAggregationJob) saveNamespaceStats(
	ctx context.Context,
	stats []*model.NamespaceGpuHourlyStats) error {

	if len(stats) == 0 {
		return nil
	}

	// 转换为数据库模型
	dbStats := make([]*dbmodel.NamespaceGpuHourlyStats, len(stats))
	for i, stat := range stats {
		dbStats[i] = convertToDBNamespaceStats(stat)
	}

	facade := database.GetFacade().GetGpuAggregation()
	if err := facade.BatchSaveNamespaceHourlyStats(ctx, dbStats); err != nil {
		return fmt.Errorf("failed to save namespace stats: %w", err)
	}

	log.Infof("Saved %d namespace stats records", len(stats))

	return nil
}

// saveLabelStats 保存label/annotation级别统计到数据库
func (j *GpuAggregationJob) saveLabelStats(
	ctx context.Context,
	stats []*model.LabelGpuHourlyStats) error {

	if len(stats) == 0 {
		return nil
	}

	// 转换为数据库模型
	dbStats := make([]*dbmodel.LabelGpuHourlyStats, len(stats))
	for i, stat := range stats {
		dbStats[i] = convertToDBLabelStats(stat)
	}

	facade := database.GetFacade().GetGpuAggregation()
	if err := facade.BatchSaveLabelHourlyStats(ctx, dbStats); err != nil {
		return fmt.Errorf("failed to save label/annotation stats: %w", err)
	}

	log.Infof("Saved %d label/annotation stats records", len(stats))

	return nil
}

// saveSnapshotToDatabase 保存快照到数据库
func (j *GpuAggregationJob) saveSnapshotToDatabase(
	ctx context.Context,
	snapshot *GpuSnapshot) error {

	// 构建详细信息JSON
	details := model.AllocationDetails{
		Namespaces:  make(map[string]model.NamespaceAllocation),
		Annotations: make(map[string]model.AnnotationAllocation),
	}

	for namespace, data := range snapshot.NamespaceData {
		utilization := 0.0
		if data.AllocatedGPU > 0 {
			utilization = data.UtilizationSum / float64(data.AllocatedGPU)
		}
		details.Namespaces[namespace] = model.NamespaceAllocation{
			AllocatedGPU:  data.AllocatedGPU,
			Utilization:   utilization,
			WorkloadCount: data.WorkloadCount,
			Workloads:     data.Workloads,
		}
	}

	for annotationKey, valueMap := range snapshot.AnnotationData {
		for annotationValue, data := range valueMap {
			key := fmt.Sprintf("%s:%s", annotationKey, annotationValue)
			utilization := 0.0
			if data.AllocatedGPU > 0 {
				utilization = data.UtilizationSum / float64(data.AllocatedGPU)
			}
			details.Annotations[key] = model.AnnotationAllocation{
				AllocatedGPU:  data.AllocatedGPU,
				Utilization:   utilization,
				WorkloadCount: data.WorkloadCount,
			}
		}
	}

	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("failed to marshal allocation details: %w", err)
	}

	// 将 JSON 解析为 ExtType (map[string]interface{})
	var allocationDetails dbmodel.ExtType
	if err := json.Unmarshal(detailsJSON, &allocationDetails); err != nil {
		return fmt.Errorf("failed to unmarshal allocation details: %w", err)
	}

	// 转换为数据库模型
	dbSnapshot := &dbmodel.GpuAllocationSnapshots{
		ClusterName:       snapshot.ClusterName,
		SnapshotTime:      snapshot.Timestamp,
		DimensionType:     "cluster", // 集群级别快照
		TotalGpuCapacity:  int32(snapshot.TotalCapacity),
		AllocatedGpuCount: int32(snapshot.AllocatedGPU),
		AllocationDetails: allocationDetails,
	}

	facade := database.GetFacade().GetGpuAggregation()
	if err := facade.SaveSnapshot(ctx, dbSnapshot); err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}

	return nil
}

// Schedule 返回Job的调度表达式
func (j *GpuAggregationJob) Schedule() string {
	// 每5分钟执行一次采样
	return "@every 5m"
}

// calculatePercentile 计算百分位数
func calculatePercentile(sortedValues []float64, percentile float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}

	index := int(float64(len(sortedValues)-1) * percentile)
	return sortedValues[index]
}

// ==================== 类型转换函数 ====================

// convertToDBClusterStats 将应用层模型转换为数据库模型
func convertToDBClusterStats(stats *model.ClusterGpuHourlyStats) *dbmodel.ClusterGpuHourlyStats {
	return &dbmodel.ClusterGpuHourlyStats{
		ClusterName:       stats.ClusterName,
		StatHour:          stats.StatHour,
		TotalGpuCapacity:  int32(stats.TotalGpuCapacity),
		AllocatedGpuCount: stats.AllocatedGpuCount,
		AllocationRate:    stats.AllocationRate,
		AvgUtilization:    stats.AvgUtilization,
		MaxUtilization:    stats.MaxUtilization,
		MinUtilization:    stats.MinUtilization,
		P50Utilization:    stats.P50Utilization,
		P95Utilization:    stats.P95Utilization,
		SampleCount:       int32(stats.SampleCount),
	}
}

// convertToDBNamespaceStats 将应用层模型转换为数据库模型
func convertToDBNamespaceStats(stats *model.NamespaceGpuHourlyStats) *dbmodel.NamespaceGpuHourlyStats {
	return &dbmodel.NamespaceGpuHourlyStats{
		ClusterName:         stats.ClusterName,
		Namespace:           stats.Namespace,
		StatHour:            stats.StatHour,
		TotalGpuCapacity:    int32(stats.TotalGpuCapacity),
		AllocatedGpuCount:   stats.AllocatedGpuCount,
		AvgUtilization:      stats.AvgUtilization,
		MaxUtilization:      stats.MaxUtilization,
		MinUtilization:      stats.MinUtilization,
		ActiveWorkloadCount: int32(stats.ActiveWorkloadCount),
	}
}

// convertToDBLabelStats 将应用层模型转换为数据库模型
func convertToDBLabelStats(stats *model.LabelGpuHourlyStats) *dbmodel.LabelGpuHourlyStats {
	return &dbmodel.LabelGpuHourlyStats{
		ClusterName:         stats.ClusterName,
		DimensionType:       stats.DimensionType,
		DimensionKey:        stats.DimensionKey,
		DimensionValue:      stats.DimensionValue,
		StatHour:            stats.StatHour,
		AllocatedGpuCount:   stats.AllocatedGpuCount,
		AvgUtilization:      stats.AvgUtilization,
		MaxUtilization:      stats.MaxUtilization,
		MinUtilization:      stats.MinUtilization,
		ActiveWorkloadCount: int32(stats.ActiveWorkloadCount),
	}
}

// getClusterGpuCapacity 从数据库获取集群GPU总容量
func (j *GpuAggregationJob) getClusterGpuCapacity(ctx context.Context, clusterName string) (int, error) {
	// 从数据库的node表查询所有GPU节点并汇总容量
	nodes, _, err := database.GetFacadeForCluster(clusterName).GetNode().
		SearchNode(ctx, filter.NodeFilter{
			// 查询所有GPU节点（GpuCount > 0）
			Limit: 10000, // 设置一个足够大的限制
		})

	if err != nil {
		return 0, fmt.Errorf("failed to query nodes from database: %w", err)
	}

	totalCapacity := 0
	gpuNodeCount := 0
	for _, node := range nodes {
		if node.GpuCount > 0 {
			totalCapacity += int(node.GpuCount)
			gpuNodeCount++
		}
	}

	log.Infof("Cluster GPU capacity from database: %d GPUs across %d nodes", totalCapacity, gpuNodeCount)
	return totalCapacity, nil
}

// buildPodToWorkloadMapping 构建Pod UID到Workload的映射关系
func (j *GpuAggregationJob) buildPodToWorkloadMapping(
	ctx context.Context,
	clusterName string,
	dbPods []*dbmodel.GpuPods) (map[string]*dbmodel.GpuWorkload, error) {

	if len(dbPods) == 0 {
		return make(map[string]*dbmodel.GpuWorkload), nil
	}

	// 收集所有Pod UIDs
	podUIDs := make([]string, 0, len(dbPods))
	for _, pod := range dbPods {
		podUIDs = append(podUIDs, pod.UID)
	}

	// 查询Pod到Workload的引用关系
	workloadRefs, err := database.GetFacadeForCluster(clusterName).GetWorkload().
		ListWorkloadPodReferencesByPodUids(ctx, podUIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to list workload pod references: %w", err)
	}

	if len(workloadRefs) == 0 {
		log.Infof("No workload references found for pods")
		return make(map[string]*dbmodel.GpuWorkload), nil
	}

	// 收集所有Workload UIDs
	workloadUIDs := make([]string, 0, len(workloadRefs))
	podToWorkloadUID := make(map[string]string)
	for _, ref := range workloadRefs {
		workloadUIDs = append(workloadUIDs, ref.WorkloadUID)
		podToWorkloadUID[ref.PodUID] = ref.WorkloadUID
	}

	// 查询最顶层的Workload信息（包含labels）
	workloads, err := database.GetFacadeForCluster(clusterName).GetWorkload().
		ListTopLevelWorkloadByUids(ctx, workloadUIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to list top level workloads: %w", err)
	}

	// 构建Workload UID到Workload的映射
	workloadUIDToWorkload := make(map[string]*dbmodel.GpuWorkload)
	for i := range workloads {
		workloadUIDToWorkload[workloads[i].UID] = workloads[i]
	}

	// 构建Pod UID到Workload的映射
	result := make(map[string]*dbmodel.GpuWorkload)
	for podUID, workloadUID := range podToWorkloadUID {
		if workload, exists := workloadUIDToWorkload[workloadUID]; exists {
			result[podUID] = workload
		}
	}

	log.Infof("Built pod to workload mapping: %d pods, %d workloads", len(result), len(workloads))
	return result, nil
}

// loadConfig 从配置管理器加载配置
func (j *GpuAggregationJob) loadConfig(ctx context.Context) error {
	var cfg model.GpuAggregationConfig

	// 从配置管理器读取配置
	err := j.configManager.Get(ctx, ConfigKeyGpuAggregation, &cfg)
	if err != nil {
		// 配置不存在
		log.Infof("GPU aggregation config not found (key: %s), job will not run. Please set config first.", ConfigKeyGpuAggregation)
		return fmt.Errorf("config not found: %w", err)
	}

	j.config = &cfg
	log.Infof("GPU aggregation config loaded successfully: enabled=%v, sampling_enabled=%v, sampling_interval=%s",
		cfg.Enabled, cfg.Sampling.Enabled, cfg.Sampling.Interval)

	return nil
}

// ReloadConfig 重新加载配置（用于配置热更新）
func (j *GpuAggregationJob) ReloadConfig(ctx context.Context) error {
	log.Infof("Reloading GPU aggregation config from config manager")
	return j.loadConfig(ctx)
}

// GetConfig 获取当前配置（只读）
func (j *GpuAggregationJob) GetConfig() *model.GpuAggregationConfig {
	return j.config
}

// UpdateConfig 更新配置到配置管理器
func (j *GpuAggregationJob) UpdateConfig(ctx context.Context, cfg *model.GpuAggregationConfig, updatedBy string) error {
	err := j.configManager.Set(ctx, ConfigKeyGpuAggregation, cfg,
		config.WithDescription("GPU使用率聚合任务配置"),
		config.WithCategory("job"),
		config.WithUpdatedBy(updatedBy),
		config.WithRecordHistory(true),
		config.WithChangeReason("Update GPU aggregation job config"),
	)
	if err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	// 更新本地配置
	j.config = cfg
	log.Infof("GPU aggregation config updated successfully by %s", updatedBy)

	return nil
}

// InitDefaultConfig 初始化默认配置（如果配置不存在）
func InitDefaultConfig(ctx context.Context, clusterName string) error {
	configManager := config.GetConfigManagerForCluster(clusterName)

	// 检查配置是否已存在
	exists, err := configManager.Exists(ctx, ConfigKeyGpuAggregation)
	if err != nil {
		return fmt.Errorf("failed to check config existence: %w", err)
	}

	if exists {
		log.Infof("GPU aggregation config already exists for cluster: %s", clusterName)
		return nil
	}

	// 创建默认配置
	defaultConfig := &model.GpuAggregationConfig{
		Enabled: true,
	}

	// 采样配置
	defaultConfig.Sampling.Enabled = true
	defaultConfig.Sampling.Interval = "5m"
	defaultConfig.Sampling.Timeout = "2m"

	// 聚合配置
	defaultConfig.Aggregation.Enabled = true
	defaultConfig.Aggregation.TriggerOffsetMinutes = 5
	defaultConfig.Aggregation.Timeout = "5m"

	// 维度配置
	defaultConfig.Dimensions.Cluster.Enabled = true

	defaultConfig.Dimensions.Namespace.Enabled = true
	defaultConfig.Dimensions.Namespace.IncludeSystemNamespaces = false
	defaultConfig.Dimensions.Namespace.ExcludeNamespaces = []string{}

	defaultConfig.Dimensions.Label.Enabled = true
	defaultConfig.Dimensions.Label.LabelKeys = []string{"app", "team", "env"}
	defaultConfig.Dimensions.Label.AnnotationKeys = []string{"project", "cost-center"}
	defaultConfig.Dimensions.Label.DefaultValue = "unknown"

	// Prometheus配置
	defaultConfig.Prometheus.WorkloadUtilizationQuery = `avg(dcgm_gpu_utilization{pod_uid="%s"})`
	defaultConfig.Prometheus.QueryStep = 30 // 30秒
	defaultConfig.Prometheus.QueryTimeout = "30s"

	// 保存默认配置
	err = configManager.Set(ctx, ConfigKeyGpuAggregation, defaultConfig,
		config.WithDescription("GPU使用率聚合任务配置"),
		config.WithCategory("job"),
		config.WithCreatedBy("system"),
		config.WithRecordHistory(true),
	)
	if err != nil {
		return fmt.Errorf("failed to save default config: %w", err)
	}

	log.Infof("Default GPU aggregation config initialized for cluster: %s", clusterName)
	return nil
}
