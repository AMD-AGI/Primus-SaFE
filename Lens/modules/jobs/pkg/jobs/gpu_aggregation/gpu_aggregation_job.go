package gpu_aggregation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/prom"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/jobs/pkg/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	// ConfigKeyGpuAggregation is the configuration key for GPU aggregation task
	ConfigKeyGpuAggregation = "job.gpu_aggregation.config"
)

// GpuAggregationJob is the GPU utilization aggregation job
type GpuAggregationJob struct {
	config        *model.GpuAggregationConfig
	snapshotCache []GpuSnapshot   // In-memory cache for snapshots of current hour
	currentHour   time.Time       // Current hour being sampled
	configManager *config.Manager // Configuration manager
	clusterName   string          // Cluster name
}

// GpuSnapshot is a GPU sampling snapshot
type GpuSnapshot struct {
	Timestamp      time.Time
	ClusterName    string
	TotalCapacity  int
	AllocatedGPU   int
	UtilizationSum float64
	ActiveGPUCount int

	// Dimension data
	NamespaceData  map[string]*NamespaceGpuData
	LabelData      map[string]map[string]*LabelGpuData // labelKey -> labelValue -> data
	AnnotationData map[string]map[string]*LabelGpuData // annotationKey -> annotationValue -> data
	WorkloadData   map[string]*WorkloadGpuData         // workloadUID -> data
}

// NamespaceGpuData is GPU data at the namespace dimension
type NamespaceGpuData struct {
	Namespace      string
	AllocatedGPU   int
	UtilizationSum float64
	WorkloadCount  int
	Workloads      []model.WorkloadSnapshot
}

// LabelGpuData is GPU data at the label/annotation dimension
type LabelGpuData struct {
	DimensionType  string // 'label' or 'annotation'
	DimensionKey   string
	DimensionValue string
	AllocatedGPU   int
	UtilizationSum float64
	WorkloadCount  int
}

// WorkloadGpuData is GPU data at the workload dimension
type WorkloadGpuData struct {
	WorkloadUID       string
	WorkloadName      string
	Namespace         string
	WorkloadType      string
	Labels            map[string]interface{}
	Annotations       map[string]interface{}
	RequestedGPU      int
	AllocatedGPU      int
	UtilizationValues []float64 // Utilization at each sampling point
	ReplicaCount      int       // Number of pods
	WorkloadStatus    string
	OwnerUID          string
	OwnerName         string
}

// NewGpuAggregationJob creates a new aggregation job
// If clusterName is empty, uses current cluster
func NewGpuAggregationJob() *GpuAggregationJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()

	return &GpuAggregationJob{
		config:        nil,                        // Config will be read from config manager at Run time
		snapshotCache: make([]GpuSnapshot, 0, 12), // 12 snapshots per hour (5-minute interval)
		currentHour:   time.Now().Truncate(time.Hour),
		configManager: config.GetConfigManagerForCluster(clusterName),
		clusterName:   clusterName,
	}
}

// NewGpuAggregationJobWithConfig creates an aggregation job with the specified config (preserved for compatibility)
func NewGpuAggregationJobWithConfig(cfg *model.GpuAggregationConfig) *GpuAggregationJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	return &GpuAggregationJob{
		config:        cfg,
		snapshotCache: make([]GpuSnapshot, 0, 12), // 12 snapshots per hour (5-minute interval)
		currentHour:   time.Now().Truncate(time.Hour),
		configManager: config.GetConfigManagerForCluster(clusterName),
		clusterName:   clusterName,
	}
}

// Run executes the job (called by job scheduler)
func (j *GpuAggregationJob) Run(ctx context.Context,
	k8sClientSet *clientsets.K8SClientSet,
	storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {

	// Create main trace span
	span, ctx := trace.StartSpanFromContext(ctx, "gpu_aggregation_job.Run")
	defer trace.FinishSpan(span)

	// Record total job start time
	jobStartTime := time.Now()

	stats := common.NewExecutionStats()

	clusterName := j.clusterName
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Set span attributes
	span.SetAttributes(
		attribute.String("job.name", "gpu_aggregation"),
		attribute.String("cluster.name", clusterName),
	)

	// If config is nil, load from config manager
	if j.config == nil {
		configSpan, configCtx := trace.StartSpanFromContext(ctx, "loadConfig")
		if err := j.loadConfig(configCtx); err != nil {
			configSpan.RecordError(err)
			configSpan.SetStatus(codes.Error, err.Error())
			trace.FinishSpan(configSpan)
			log.Warnf("Failed to load GPU aggregation config, job will not run: %v", err)
			stats.AddMessage("GPU aggregation config not found, job disabled")
			totalDuration := time.Since(jobStartTime)
			span.SetAttributes(attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())))
			span.SetStatus(codes.Ok, "Config not found, job disabled")
			return stats, nil // Return nil to allow scheduler to continue running
		}
		configSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(configSpan)
	}

	// Check if config is enabled
	if !j.config.Enabled {
		log.Debugf("GPU aggregation job is disabled in config")
		stats.AddMessage("GPU aggregation job is disabled in config")
		totalDuration := time.Since(jobStartTime)
		span.SetAttributes(
			attribute.Bool("config.enabled", false),
			attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())),
		)
		span.SetStatus(codes.Ok, "Job disabled in config")
		return stats, nil
	}

	span.SetAttributes(attribute.Bool("config.enabled", true))

	// Check if hourly aggregation needs to be performed
	now := time.Now()
	currentHour := now.Truncate(time.Hour)

	// If hour has changed, aggregate data for the previous hour from database
	if currentHour.After(j.currentHour) {
		log.Infof("Hour changed, aggregating data for hour: %v from database", j.currentHour)
		aggStart := time.Now()

		aggSpan, aggCtx := trace.StartSpanFromContext(ctx, "aggregateHourlyData")
		aggSpan.SetAttributes(
			attribute.String("cluster.name", clusterName),
			attribute.String("stat.hour", j.currentHour.Format(time.RFC3339)),
		)

		if err := j.aggregateHourlyDataFromDB(aggCtx, clusterName, j.currentHour); err != nil {
			aggSpan.RecordError(err)
			aggSpan.SetAttributes(
				attribute.String("error.message", err.Error()),
			)
			aggSpan.SetStatus(codes.Error, err.Error())
			trace.FinishSpan(aggSpan)

			stats.ErrorCount++
			log.Errorf("Failed to aggregate hourly data: %v", err)
			// Don't return error, continue sampling
		} else {
			duration := time.Since(aggStart)
			aggSpan.SetAttributes(
				attribute.Float64("duration_ms", float64(duration.Milliseconds())),
			)
			aggSpan.SetStatus(codes.Ok, "")
			trace.FinishSpan(aggSpan)

			stats.ProcessDuration += duration.Seconds()
			stats.ItemsCreated++ // Created one hourly aggregation record
			stats.AddMessage(fmt.Sprintf("Aggregated hourly data for %v from database", j.currentHour))
		}

		// Update current hour
		j.currentHour = currentHour
	}

	// Perform sampling
	if j.config.Sampling.Enabled {
		sampleStart := time.Now()

		sampleSpan, sampleCtx := trace.StartSpanFromContext(ctx, "sample")
		sampleSpan.SetAttributes(
			attribute.String("cluster.name", clusterName),
		)

		if err := j.sample(sampleCtx, clusterName, k8sClientSet, storageClientSet); err != nil {
			sampleSpan.RecordError(err)
			sampleSpan.SetAttributes(
				attribute.String("error.message", err.Error()),
			)
			sampleSpan.SetStatus(codes.Error, err.Error())
			trace.FinishSpan(sampleSpan)

			log.Errorf("Failed to sample GPU data: %v", err)
			span.SetStatus(codes.Error, "Sampling failed")
			return stats, err
		}

		duration := time.Since(sampleStart)
		sampleSpan.SetAttributes(
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
			attribute.Int("snapshots.cached", len(j.snapshotCache)),
		)
		sampleSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(sampleSpan)

		stats.QueryDuration = duration.Seconds()
		stats.RecordsProcessed = int64(len(j.snapshotCache))
		stats.AddCustomMetric("snapshots_cached", len(j.snapshotCache))
		stats.AddMessage("GPU data sampled successfully")
	}

	// Record total job duration
	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())))
	span.SetStatus(codes.Ok, "")
	return stats, nil
}

// sample samples the current GPU state
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
		WorkloadData:   make(map[string]*WorkloadGpuData),
	}

	// 1. Get cluster GPU total capacity from database
	capacitySpan, capacityCtx := trace.StartSpanFromContext(ctx, "getClusterGpuCapacity")
	capacitySpan.SetAttributes(attribute.String("cluster.name", clusterName))

	totalCapacity, err := j.getClusterGpuCapacity(capacityCtx, clusterName)
	if err != nil {
		capacitySpan.RecordError(err)
		capacitySpan.SetAttributes(attribute.String("error.message", err.Error()))
		capacitySpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(capacitySpan)
		return fmt.Errorf("failed to get GPU capacity: %w", err)
	}
	capacitySpan.SetAttributes(attribute.Int("gpu.total_capacity", totalCapacity))
	capacitySpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(capacitySpan)

	snapshot.TotalCapacity = totalCapacity

	// 2. Get all active GPU pods from database
	podsSpan, podsCtx := trace.StartSpanFromContext(ctx, "listActiveGpuPods")
	podsSpan.SetAttributes(attribute.String("cluster.name", clusterName))

	dbPods, err := database.GetFacadeForCluster(clusterName).GetPod().ListActiveGpuPods(podsCtx)
	if err != nil {
		podsSpan.RecordError(err)
		podsSpan.SetAttributes(attribute.String("error.message", err.Error()))
		podsSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(podsSpan)
		return fmt.Errorf("failed to list active GPU pods from database: %w", err)
	}
	podsSpan.SetAttributes(attribute.Int("pods.count", len(dbPods)))
	podsSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(podsSpan)

	log.Infof("Found %d active GPU pods in database", len(dbPods))

	// 3. Process each pod, collect GPU allocation and utilization
	allocatedGPU := 0
	utilizationSum := 0.0
	activeGPUCount := 0

	// Build PodUID -> Workload mapping
	mappingSpan, mappingCtx := trace.StartSpanFromContext(ctx, "buildPodToWorkloadMapping")
	mappingSpan.SetAttributes(
		attribute.String("cluster.name", clusterName),
		attribute.Int("pods.count", len(dbPods)),
	)

	podUIDToWorkload, err := j.buildPodToWorkloadMapping(mappingCtx, clusterName, dbPods)
	if err != nil {
		mappingSpan.RecordError(err)
		mappingSpan.SetAttributes(attribute.String("error.message", err.Error()))
		mappingSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(mappingSpan)

		log.Warnf("Failed to build pod to workload mapping: %v", err)
		// Continue processing even without workload information
	} else {
		mappingSpan.SetAttributes(attribute.Int("workload.mapping_count", len(podUIDToWorkload)))
		mappingSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(mappingSpan)
	}

	// Process pod data collection
	collectSpan, collectCtx := trace.StartSpanFromContext(ctx, "collectPodData")
	collectSpan.SetAttributes(
		attribute.Int("pods.count", len(dbPods)),
	)

	utilizationQueryCount := 0
	utilizationErrorCount := 0

	for _, dbPod := range dbPods {
		gpuRequest := int(dbPod.GpuAllocated)
		if gpuRequest == 0 {
			continue
		}

		allocatedGPU += gpuRequest

		// Query GPU utilization for this pod from Prometheus
		utilization, err := j.queryWorkloadUtilization(collectCtx, storageClientSet, dbPod.UID)
		utilizationQueryCount++
		if err != nil {
			utilizationErrorCount++
			log.Warnf("Failed to query utilization for pod %s: %v", dbPod.UID, err)
			utilization = 0 // Use 0 when query fails
		}

		utilizationSum += utilization * float64(gpuRequest)
		activeGPUCount += gpuRequest

		// Get workload information associated with this pod (for labels and annotations)
		workload := podUIDToWorkload[dbPod.UID]

		// 4. Collect namespace dimension data
		j.collectNamespaceDataFromDB(&snapshot, dbPod, workload, gpuRequest, utilization)

		// 5. Collect label dimension data
		j.collectLabelDataFromDB(&snapshot, dbPod, workload, gpuRequest, utilization)

		// 6. Collect annotation dimension data
		j.collectAnnotationDataFromDB(&snapshot, dbPod, workload, gpuRequest, utilization)

		// 7. Collect workload dimension data
		j.collectWorkloadDataFromDB(&snapshot, dbPod, workload, gpuRequest, utilization)
	}

	collectSpan.SetAttributes(
		attribute.Int("gpu.allocated", allocatedGPU),
		attribute.Int("gpu.active_count", activeGPUCount),
		attribute.Int("utilization.query_count", utilizationQueryCount),
		attribute.Int("utilization.error_count", utilizationErrorCount),
	)
	collectSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(collectSpan)

	snapshot.AllocatedGPU = allocatedGPU
	snapshot.UtilizationSum = utilizationSum
	snapshot.ActiveGPUCount = activeGPUCount

	// 6. Save snapshot to cache
	j.snapshotCache = append(j.snapshotCache, snapshot)

	// 7. Save snapshot to database (optional, for debugging and auditing)
	saveSpan, saveCtx := trace.StartSpanFromContext(ctx, "saveSnapshotToDatabase")
	saveSpan.SetAttributes(
		attribute.String("cluster.name", clusterName),
		attribute.Int("gpu.allocated", allocatedGPU),
		attribute.Int("gpu.total_capacity", totalCapacity),
	)

	if err := j.saveSnapshotToDatabase(saveCtx, &snapshot); err != nil {
		saveSpan.RecordError(err)
		saveSpan.SetAttributes(attribute.String("error.message", err.Error()))
		saveSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(saveSpan)

		log.Warnf("Failed to save snapshot to database: %v", err)
		// Don't return error, snapshot save failure doesn't affect sampling
	} else {
		saveSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(saveSpan)
	}

	duration := time.Since(startTime)
	log.Infof("GPU usage sampling completed for cluster: %s, took: %v, allocated: %d/%d GPUs",
		clusterName, duration, allocatedGPU, totalCapacity)

	// TODO: Export Prometheus metrics
	// j.exportMetrics(&snapshot)

	return nil
}

// collectNamespaceDataFromDB collects namespace dimension data from database pods
func (j *GpuAggregationJob) collectNamespaceDataFromDB(
	snapshot *GpuSnapshot,
	dbPod *dbmodel.GpuPods,
	workload *dbmodel.GpuWorkload,
	gpuRequest int,
	utilization float64) {

	namespace := dbPod.Namespace

	// Check if this namespace should be excluded
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

	// Record workload information
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

// collectLabelDataFromDB collects label dimension data from database workloads
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
		// Cannot get labels without workload information
		return
	}

	// Get labels from workload's Labels field (ExtType/map[string]interface{})
	labels := make(map[string]string)
	if workload.Labels != nil {
		for k, v := range workload.Labels {
			if strVal, ok := v.(string); ok {
				labels[k] = strVal
			}
		}
	}

	// Iterate through configured label keys
	for _, labelKey := range j.config.Dimensions.Label.LabelKeys {
		labelValue := labels[labelKey]
		if labelValue == "" {
			labelValue = j.config.Dimensions.Label.DefaultValue
		}

		// Ensure labelKey's map exists
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

// collectAnnotationDataFromDB collects annotation dimension data from database
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
		// Cannot get annotations without workload information
		return
	}

	// Get annotations from workload's Annotations field (ExtType/map[string]interface{})
	annotations := make(map[string]string)
	if workload.Annotations != nil {
		for k, v := range workload.Annotations {
			if strVal, ok := v.(string); ok {
				annotations[k] = strVal
			}
		}
	}

	// Iterate through configured annotation keys
	for _, annotationKey := range j.config.Dimensions.Label.AnnotationKeys {
		annotationValue := annotations[annotationKey]
		if annotationValue == "" {
			annotationValue = j.config.Dimensions.Label.DefaultValue
		}

		// Ensure annotationKey's map exists
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

// collectWorkloadDataFromDB collects workload dimension data from database
func (j *GpuAggregationJob) collectWorkloadDataFromDB(
	snapshot *GpuSnapshot,
	dbPod *dbmodel.GpuPods,
	workload *dbmodel.GpuWorkload,
	gpuRequest int,
	utilization float64) {

	if workload == nil {
		// Cannot collect without workload information
		return
	}

	// Only collect top-level workloads (workloads without parent)
	if workload.ParentUID != "" {
		return
	}

	// Check if this namespace should be excluded
	if j.shouldExcludeNamespace(dbPod.Namespace) {
		return
	}

	workloadUID := workload.UID
	wData, exists := snapshot.WorkloadData[workloadUID]
	if !exists {
		wData = &WorkloadGpuData{
			WorkloadUID:       workload.UID,
			WorkloadName:      workload.Name,
			Namespace:         workload.Namespace,
			WorkloadType:      workload.Kind,
			Labels:            workload.Labels,
			Annotations:       workload.Annotations,
			RequestedGPU:      0,
			AllocatedGPU:      0,
			UtilizationValues: make([]float64, 0),
			ReplicaCount:      0,
			WorkloadStatus:    workload.Status,
			OwnerUID:          workload.ParentUID, // Use ParentUID as OwnerUID
			OwnerName:         "",                 // GpuWorkload doesn't have OwnerName field
		}
		snapshot.WorkloadData[workloadUID] = wData
	}

	wData.AllocatedGPU += gpuRequest
	wData.RequestedGPU += gpuRequest // Assume requested and allocated are the same
	wData.UtilizationValues = append(wData.UtilizationValues, utilization)
	wData.ReplicaCount++ // Each pod counts as one replica
}

// shouldExcludeNamespace determines whether this namespace should be excluded
func (j *GpuAggregationJob) shouldExcludeNamespace(namespace string) bool {
	if !j.config.Dimensions.Namespace.Enabled {
		return true
	}

	// Check if in exclusion list
	for _, excluded := range j.config.Dimensions.Namespace.ExcludeNamespaces {
		if namespace == excluded {
			return true
		}
	}

	// Check if it's a system namespace
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

// queryWorkloadUtilization queries the GPU utilization of a workload
func (j *GpuAggregationJob) queryWorkloadUtilization(
	ctx context.Context,
	storageClientSet *clientsets.StorageClientSet,
	workloadUID string) (float64, error) {

	span, ctx := trace.StartSpanFromContext(ctx, "queryWorkloadUtilization")
	defer trace.FinishSpan(span)

	// Use configured query template
	query := fmt.Sprintf(j.config.Prometheus.WorkloadUtilizationQuery, workloadUID)

	span.SetAttributes(
		attribute.String("workload.uid", workloadUID),
		attribute.String("prometheus.query", query),
		attribute.Int("prometheus.query_step", j.config.Prometheus.QueryStep),
	)

	// Query average for the last 1 minute
	endTime := time.Now()
	startTime := endTime.Add(-1 * time.Minute)

	series, err := prom.QueryRange(ctx, storageClientSet, query, startTime, endTime,
		j.config.Prometheus.QueryStep, map[string]struct{}{"__name__": {}})

	if err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.String("error.message", err.Error()))
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}

	if len(series) == 0 || len(series[0].Values) == 0 {
		span.SetAttributes(
			attribute.Int("series.count", 0),
			attribute.Float64("utilization.avg", 0),
		)
		span.SetStatus(codes.Ok, "No data points")
		return 0, nil
	}

	// Calculate average
	sum := 0.0
	for _, point := range series[0].Values {
		sum += point.Value
	}
	avg := sum / float64(len(series[0].Values))

	span.SetAttributes(
		attribute.Int("series.count", len(series)),
		attribute.Int("data_points.count", len(series[0].Values)),
		attribute.Float64("utilization.avg", avg),
	)
	span.SetStatus(codes.Ok, "")

	return avg, nil
}

// aggregateHourlyDataFromDB aggregates hourly data by loading snapshots from database
func (j *GpuAggregationJob) aggregateHourlyDataFromDB(
	ctx context.Context,
	clusterName string,
	hour time.Time) error {

	// Load snapshots from database for the specified hour
	startTime := hour
	endTime := hour.Add(time.Hour)

	log.Infof("Loading snapshots from database for hour: %v (from %v to %v)", hour, startTime, endTime)

	loadSpan, loadCtx := trace.StartSpanFromContext(ctx, "loadSnapshotsFromDB")
	loadSpan.SetAttributes(
		attribute.String("cluster.name", clusterName),
		attribute.String("start_time", startTime.Format(time.RFC3339)),
		attribute.String("end_time", endTime.Format(time.RFC3339)),
	)

	facade := database.GetFacadeForCluster(clusterName).GetGpuAggregation()
	dbSnapshots, err := facade.ListSnapshots(loadCtx, startTime, endTime)
	if err != nil {
		loadSpan.RecordError(err)
		loadSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(loadSpan)
		return fmt.Errorf("failed to load snapshots from database: %w", err)
	}

	loadSpan.SetAttributes(attribute.Int("snapshots.count", len(dbSnapshots)))
	loadSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(loadSpan)

	if len(dbSnapshots) == 0 {
		log.Warnf("No snapshots found in database for hour: %v", hour)
		return nil
	}

	log.Infof("Loaded %d snapshots from database for hour: %v", len(dbSnapshots), hour)

	// Convert database snapshots to memory format
	convertSpan, convertCtx := trace.StartSpanFromContext(ctx, "convertSnapshots")
	snapshots, err := j.convertDBSnapshotsToMemory(convertCtx, dbSnapshots)
	if err != nil {
		convertSpan.RecordError(err)
		convertSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(convertSpan)
		return fmt.Errorf("failed to convert snapshots: %w", err)
	}
	convertSpan.SetAttributes(attribute.Int("converted.count", len(snapshots)))
	convertSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(convertSpan)

	// Store converted snapshots temporarily for aggregation
	originalCache := j.snapshotCache
	j.snapshotCache = snapshots
	defer func() {
		j.snapshotCache = originalCache
	}()

	// Perform aggregation using existing logic
	return j.aggregateHourlyData(ctx, clusterName, hour)
}

// aggregateHourlyData aggregates hourly data from in-memory snapshots
func (j *GpuAggregationJob) aggregateHourlyData(
	ctx context.Context,
	clusterName string,
	hour time.Time) error {

	if len(j.snapshotCache) == 0 {
		log.Warnf("No snapshots to aggregate for hour: %v", hour)
		return nil
	}

	log.Infof("Aggregating %d snapshots for hour: %v", len(j.snapshotCache), hour)
	aggStartTime := time.Now()

	// 1. Aggregate cluster-level data
	if j.config.Dimensions.Cluster.Enabled {
		clusterSpan, clusterCtx := trace.StartSpanFromContext(ctx, "aggregateClusterStats")
		clusterSpan.SetAttributes(
			attribute.String("cluster.name", clusterName),
			attribute.String("stat.hour", hour.Format(time.RFC3339)),
			attribute.Int("snapshot.count", len(j.snapshotCache)),
		)

		clusterStats := j.aggregateClusterStats(clusterName, hour)

		if err := j.saveClusterStats(clusterCtx, clusterStats); err != nil {
			clusterSpan.RecordError(err)
			clusterSpan.SetAttributes(attribute.String("error.message", err.Error()))
			clusterSpan.SetStatus(codes.Error, err.Error())
			trace.FinishSpan(clusterSpan)

			log.Errorf("Failed to save cluster stats: %v", err)
			return err
		}

		clusterSpan.SetAttributes(
			attribute.Float64("allocation_rate", clusterStats.AllocationRate),
			attribute.Float64("avg_utilization", clusterStats.AvgUtilization),
		)
		clusterSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(clusterSpan)
	}

	// 2. Aggregate namespace-level data
	if j.config.Dimensions.Namespace.Enabled {
		nsSpan, nsCtx := trace.StartSpanFromContext(ctx, "aggregateNamespaceStats")
		nsSpan.SetAttributes(
			attribute.String("cluster.name", clusterName),
			attribute.String("stat.hour", hour.Format(time.RFC3339)),
		)

		namespaceStats := j.aggregateNamespaceStats(clusterName, hour)
		nsSpan.SetAttributes(attribute.Int("namespace.stats_count", len(namespaceStats)))

		if err := j.saveNamespaceStats(nsCtx, namespaceStats); err != nil {
			nsSpan.RecordError(err)
			nsSpan.SetAttributes(attribute.String("error.message", err.Error()))
			nsSpan.SetStatus(codes.Error, err.Error())
			trace.FinishSpan(nsSpan)

			log.Errorf("Failed to save namespace stats: %v", err)
			return err
		}

		nsSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(nsSpan)
	}

	// 3. Aggregate label/annotation level data
	if j.config.Dimensions.Label.Enabled {
		labelSpan, labelCtx := trace.StartSpanFromContext(ctx, "aggregateLabelStats")
		labelSpan.SetAttributes(
			attribute.String("cluster.name", clusterName),
			attribute.String("stat.hour", hour.Format(time.RFC3339)),
		)

		labelStats := j.aggregateLabelStats(clusterName, hour)
		labelSpan.SetAttributes(attribute.Int("label.stats_count", len(labelStats)))

		if err := j.saveLabelStats(labelCtx, labelStats); err != nil {
			labelSpan.RecordError(err)
			labelSpan.SetAttributes(attribute.String("error.message", err.Error()))
			labelSpan.SetStatus(codes.Error, err.Error())
			trace.FinishSpan(labelSpan)

			log.Errorf("Failed to save label/annotation stats: %v", err)
			return err
		}

		labelSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(labelSpan)
	}

	// 4. Aggregate workload-level data
	workloadSpan, workloadCtx := trace.StartSpanFromContext(ctx, "aggregateWorkloadStats")
	workloadSpan.SetAttributes(
		attribute.String("cluster.name", clusterName),
		attribute.String("stat.hour", hour.Format(time.RFC3339)),
	)

	workloadStats := j.aggregateWorkloadStats(clusterName, hour)
	workloadSpan.SetAttributes(attribute.Int("workload.stats_count", len(workloadStats)))

	if err := j.saveWorkloadStats(workloadCtx, workloadStats); err != nil {
		workloadSpan.RecordError(err)
		workloadSpan.SetAttributes(attribute.String("error.message", err.Error()))
		workloadSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(workloadSpan)

		log.Errorf("Failed to save workload stats: %v", err)
		return err
	}

	workloadSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(workloadSpan)

	duration := time.Since(aggStartTime)
	log.Infof("Hourly aggregation completed for hour: %v, took: %v", hour, duration)

	return nil
}

// convertDBSnapshotsToMemory converts database snapshots to in-memory GpuSnapshot format
func (j *GpuAggregationJob) convertDBSnapshotsToMemory(
	ctx context.Context,
	dbSnapshots []*dbmodel.GpuAllocationSnapshots) ([]GpuSnapshot, error) {

	snapshots := make([]GpuSnapshot, 0, len(dbSnapshots))

	for _, dbSnapshot := range dbSnapshots {
		snapshot := GpuSnapshot{
			Timestamp:      dbSnapshot.SnapshotTime,
			ClusterName:    dbSnapshot.ClusterName,
			TotalCapacity:  int(dbSnapshot.TotalGpuCapacity),
			AllocatedGPU:   int(dbSnapshot.AllocatedGpuCount),
			NamespaceData:  make(map[string]*NamespaceGpuData),
			LabelData:      make(map[string]map[string]*LabelGpuData),
			AnnotationData: make(map[string]map[string]*LabelGpuData),
			WorkloadData:   make(map[string]*WorkloadGpuData),
		}

		// Parse allocation details from ExtType (map[string]interface{})
		// First marshal to JSON, then unmarshal to AllocationDetails
		detailsJSON, err := json.Marshal(dbSnapshot.AllocationDetails)
		if err != nil {
			log.Warnf("Failed to marshal allocation details for snapshot at %v: %v", dbSnapshot.SnapshotTime, err)
			continue
		}

		var details model.AllocationDetails
		if err := json.Unmarshal(detailsJSON, &details); err != nil {
			log.Warnf("Failed to parse allocation details for snapshot at %v: %v", dbSnapshot.SnapshotTime, err)
			continue
		}

		// Convert namespace data
		for namespace, nsAlloc := range details.Namespaces {
			nsData := &NamespaceGpuData{
				Namespace:      namespace,
				AllocatedGPU:   nsAlloc.AllocatedGPU,
				UtilizationSum: nsAlloc.Utilization * float64(nsAlloc.AllocatedGPU),
				WorkloadCount:  nsAlloc.WorkloadCount,
				Workloads:      nsAlloc.Workloads,
			}
			snapshot.NamespaceData[namespace] = nsData
		}

		// Convert annotation data (stored as "key:value" -> AnnotationAllocation)
		for key, annAlloc := range details.Annotations {
			// Parse "key:value" format
			parts := splitAnnotationKey(key)
			if len(parts) != 2 {
				log.Warnf("Invalid annotation key format: %s", key)
				continue
			}
			annotationKey := parts[0]
			annotationValue := parts[1]

			if _, exists := snapshot.AnnotationData[annotationKey]; !exists {
				snapshot.AnnotationData[annotationKey] = make(map[string]*LabelGpuData)
			}

			annData := &LabelGpuData{
				DimensionType:  "annotation",
				DimensionKey:   annotationKey,
				DimensionValue: annotationValue,
				AllocatedGPU:   annAlloc.AllocatedGPU,
				UtilizationSum: annAlloc.Utilization * float64(annAlloc.AllocatedGPU),
				WorkloadCount:  annAlloc.WorkloadCount,
			}
			snapshot.AnnotationData[annotationKey][annotationValue] = annData
		}

		// Convert workload data
		for workloadUID, wSnapshot := range details.Workloads {
			wData := &WorkloadGpuData{
				WorkloadUID:       workloadUID,
				WorkloadName:      wSnapshot.Name,
				Namespace:         wSnapshot.Namespace,
				WorkloadType:      wSnapshot.Kind,
				AllocatedGPU:      wSnapshot.AllocatedGPU,
				UtilizationValues: []float64{wSnapshot.Utilization}, // Store as single value
				ReplicaCount:      1,
			}
			snapshot.WorkloadData[workloadUID] = wData
		}

		// Calculate utilization sum and active GPU count
		utilizationSum := 0.0
		activeGPUCount := 0
		for _, nsData := range snapshot.NamespaceData {
			utilizationSum += nsData.UtilizationSum
			activeGPUCount += nsData.AllocatedGPU
		}
		snapshot.UtilizationSum = utilizationSum
		snapshot.ActiveGPUCount = activeGPUCount

		snapshots = append(snapshots, snapshot)
	}

	log.Infof("Converted %d database snapshots to memory format", len(snapshots))
	return snapshots, nil
}

// splitAnnotationKey splits "key:value" into [key, value]
func splitAnnotationKey(s string) []string {
	idx := strings.Index(s, ":")
	if idx == -1 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+1:]}
}

// aggregateClusterStats aggregates cluster-level statistics
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

		// Calculate average utilization for this snapshot
		var utilization float64
		if snapshot.ActiveGPUCount > 0 {
			utilization = snapshot.UtilizationSum / float64(snapshot.ActiveGPUCount)
		}
		utilizationValues = append(utilizationValues, utilization)
	}

	// Calculate averages
	count := float64(len(j.snapshotCache))
	stats.TotalGpuCapacity = int(float64(totalCapacitySum) / count)
	stats.AllocatedGpuCount = float64(allocatedSum) / count

	if stats.TotalGpuCapacity > 0 {
		stats.AllocationRate = (stats.AllocatedGpuCount / float64(stats.TotalGpuCapacity)) * 100
	}

	// Calculate utilization statistics
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

// aggregateNamespaceStats aggregates namespace-level statistics
func (j *GpuAggregationJob) aggregateNamespaceStats(
	clusterName string,
	hour time.Time) []*model.NamespaceGpuHourlyStats {

	// First aggregate data for the same namespace across all snapshots
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

			// Calculate average utilization for this namespace in this snapshot
			var nsUtilization float64
			if data.AllocatedGPU > 0 {
				nsUtilization = data.UtilizationSum / float64(data.AllocatedGPU)
			}
			agg.utilizationValues = append(agg.utilizationValues, nsUtilization)
		}
	}

	// Convert to database model
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

		// Calculate utilization statistics
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

// aggregateLabelStats aggregates label and annotation level statistics
func (j *GpuAggregationJob) aggregateLabelStats(
	clusterName string,
	hour time.Time) []*model.LabelGpuHourlyStats {

	// dimensionType -> dimensionKey -> dimensionValue -> aggregate
	labelAggregates := make(map[string]map[string]map[string]*labelAggregate)

	// Aggregate label data
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

		// Aggregate annotation data
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

	// Convert to database model
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

				// Calculate utilization statistics
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

// aggregateWorkloadStats aggregates workload-level statistics
func (j *GpuAggregationJob) aggregateWorkloadStats(
	clusterName string,
	hour time.Time) []*dbmodel.WorkloadGpuHourlyStats {

	// workloadUID -> aggregate
	workloadAggregates := make(map[string]*workloadAggregate)

	// Iterate through all snapshots and aggregate data for the same workload
	for _, snapshot := range j.snapshotCache {
		for workloadUID, data := range snapshot.WorkloadData {
			agg, exists := workloadAggregates[workloadUID]
			if !exists {
				agg = &workloadAggregate{
					workloadUID:       workloadUID,
					workloadName:      data.WorkloadName,
					namespace:         data.Namespace,
					workloadType:      data.WorkloadType,
					labels:            data.Labels,
					annotations:       data.Annotations,
					allocatedSum:      0,
					requestedSum:      0,
					utilizationValues: make([]float64, 0),
					replicaCounts:     make([]int, 0),
					workloadStatus:    data.WorkloadStatus,
					ownerUID:          data.OwnerUID,
					ownerName:         data.OwnerName,
				}
				workloadAggregates[workloadUID] = agg
			}

			agg.allocatedSum += data.AllocatedGPU
			agg.requestedSum += data.RequestedGPU
			agg.replicaCounts = append(agg.replicaCounts, data.ReplicaCount)

			// Merge utilization data for this workload in this snapshot
			agg.utilizationValues = append(agg.utilizationValues, data.UtilizationValues...)
		}
	}

	// Convert to database model
	results := make([]*dbmodel.WorkloadGpuHourlyStats, 0, len(workloadAggregates))
	count := float64(len(j.snapshotCache))

	for _, agg := range workloadAggregates {
		stats := &dbmodel.WorkloadGpuHourlyStats{
			ClusterName:       clusterName,
			Namespace:         agg.namespace,
			WorkloadName:      agg.workloadName,
			WorkloadType:      agg.workloadType,
			StatHour:          hour,
			AllocatedGpuCount: float64(agg.allocatedSum) / count,
			RequestedGpuCount: float64(agg.requestedSum) / count,
			WorkloadStatus:    agg.workloadStatus,
			SampleCount:       int32(len(agg.utilizationValues)),
			OwnerUID:          agg.ownerUID,
			OwnerName:         agg.ownerName,
		}

		// Convert labels and annotations to ExtType
		if agg.labels != nil {
			stats.Labels = dbmodel.ExtType(agg.labels)
		} else {
			stats.Labels = dbmodel.ExtType{}
		}

		if agg.annotations != nil {
			stats.Annotations = dbmodel.ExtType(agg.annotations)
		} else {
			stats.Annotations = dbmodel.ExtType{}
		}

		// Calculate utilization statistics
		sort.Float64s(agg.utilizationValues)
		if len(agg.utilizationValues) > 0 {
			stats.MinUtilization = agg.utilizationValues[0]
			stats.MaxUtilization = agg.utilizationValues[len(agg.utilizationValues)-1]
			stats.P50Utilization = calculatePercentile(agg.utilizationValues, 0.50)
			stats.P95Utilization = calculatePercentile(agg.utilizationValues, 0.95)

			utilizationSum := 0.0
			for _, v := range agg.utilizationValues {
				utilizationSum += v
			}
			stats.AvgUtilization = utilizationSum / float64(len(agg.utilizationValues))
		}

		// Calculate replica statistics
		if len(agg.replicaCounts) > 0 {
			replicaSum := 0
			maxReplica := 0
			minReplica := agg.replicaCounts[0]

			for _, r := range agg.replicaCounts {
				replicaSum += r
				if r > maxReplica {
					maxReplica = r
				}
				if r < minReplica {
					minReplica = r
				}
			}

			stats.AvgReplicaCount = float64(replicaSum) / float64(len(agg.replicaCounts))
			stats.MaxReplicaCount = int32(maxReplica)
			stats.MinReplicaCount = int32(minReplica)
		}

		// TODO: Add GPU memory related statistics (requires Prometheus query)
		// Set to 0 for now
		stats.AvgGpuMemoryUsed = 0
		stats.MaxGpuMemoryUsed = 0
		stats.AvgGpuMemoryTotal = 0

		results = append(results, stats)
	}

	return results
}

// Helper structs
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

type workloadAggregate struct {
	workloadUID       string
	workloadName      string
	namespace         string
	workloadType      string
	labels            map[string]interface{}
	annotations       map[string]interface{}
	allocatedSum      int
	requestedSum      int
	utilizationValues []float64
	replicaCounts     []int
	workloadStatus    string
	ownerUID          string
	ownerName         string
}

// saveClusterStats saves cluster-level statistics to database
func (j *GpuAggregationJob) saveClusterStats(
	ctx context.Context,
	stats *model.ClusterGpuHourlyStats) error {

	// Convert to database model
	dbStats := convertToDBClusterStats(stats)

	facade := database.GetFacade().GetGpuAggregation()
	if err := facade.SaveClusterHourlyStats(ctx, dbStats); err != nil {
		return fmt.Errorf("failed to save cluster stats: %w", err)
	}

	log.Infof("Cluster stats saved for %s at %v: allocation=%.2f%%, utilization=%.2f%%",
		stats.ClusterName, stats.StatHour, stats.AllocationRate, stats.AvgUtilization)

	return nil
}

// saveNamespaceStats saves namespace-level statistics to database
func (j *GpuAggregationJob) saveNamespaceStats(
	ctx context.Context,
	stats []*model.NamespaceGpuHourlyStats) error {

	if len(stats) == 0 {
		return nil
	}

	// Convert to database model
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

// saveLabelStats saves label/annotation level statistics to database
func (j *GpuAggregationJob) saveLabelStats(
	ctx context.Context,
	stats []*model.LabelGpuHourlyStats) error {

	if len(stats) == 0 {
		return nil
	}

	// Convert to database model
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

// saveWorkloadStats saves workload-level statistics to database
func (j *GpuAggregationJob) saveWorkloadStats(
	ctx context.Context,
	stats []*dbmodel.WorkloadGpuHourlyStats) error {

	if len(stats) == 0 {
		return nil
	}

	facade := database.GetFacade().GetGpuAggregation()
	if err := facade.BatchSaveWorkloadHourlyStats(ctx, stats); err != nil {
		return fmt.Errorf("failed to save workload stats: %w", err)
	}

	log.Infof("Saved %d workload stats records", len(stats))

	return nil
}

// saveSnapshotToDatabase saves snapshot to database
func (j *GpuAggregationJob) saveSnapshotToDatabase(
	ctx context.Context,
	snapshot *GpuSnapshot) error {

	// Build allocation details JSON
	details := model.AllocationDetails{
		Namespaces:  make(map[string]model.NamespaceAllocation),
		Annotations: make(map[string]model.AnnotationAllocation),
		Workloads:   make(map[string]model.WorkloadSnapshot),
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

	for workloadUID, data := range snapshot.WorkloadData {
		utilization := 0.0
		// Calculate average utilization from UtilizationValues
		if len(data.UtilizationValues) > 0 {
			sum := 0.0
			for _, val := range data.UtilizationValues {
				sum += val
			}
			utilization = sum / float64(len(data.UtilizationValues))
		}
		details.Workloads[workloadUID] = model.WorkloadSnapshot{
			UID:          workloadUID,
			Name:         data.WorkloadName,
			Namespace:    data.Namespace,
			Kind:         data.WorkloadType,
			AllocatedGPU: data.AllocatedGPU,
			Utilization:  utilization,
		}
	}

	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("failed to marshal allocation details: %w", err)
	}

	// Parse JSON into ExtType (map[string]interface{})
	var allocationDetails dbmodel.ExtType
	if err := json.Unmarshal(detailsJSON, &allocationDetails); err != nil {
		return fmt.Errorf("failed to unmarshal allocation details: %w", err)
	}

	// Convert to database model
	dbSnapshot := &dbmodel.GpuAllocationSnapshots{
		ClusterName:       snapshot.ClusterName,
		SnapshotTime:      snapshot.Timestamp,
		DimensionType:     "cluster", // Cluster-level snapshot
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

// Schedule returns the job's scheduling expression
func (j *GpuAggregationJob) Schedule() string {
	// Execute sampling every 5 minutes
	return "@every 5m"
}

// calculatePercentile calculates percentile value
func calculatePercentile(sortedValues []float64, percentile float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}

	index := int(float64(len(sortedValues)-1) * percentile)
	return sortedValues[index]
}

// ==================== Type Conversion Functions ====================

// convertToDBClusterStats converts application layer model to database model
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

// convertToDBNamespaceStats converts application layer model to database model
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

// convertToDBLabelStats converts application layer model to database model
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

// getClusterGpuCapacity gets cluster GPU total capacity from database
func (j *GpuAggregationJob) getClusterGpuCapacity(ctx context.Context, clusterName string) (int, error) {
	span, ctx := trace.StartSpanFromContext(ctx, "getClusterGpuCapacity.query")
	defer trace.FinishSpan(span)

	span.SetAttributes(attribute.String("cluster.name", clusterName))

	// Query all GPU nodes from database node table and sum capacity
	readyStatus := "Ready"
	nodes, _, err := database.GetFacadeForCluster(clusterName).GetNode().
		SearchNode(ctx, filter.NodeFilter{
			// Query all GPU nodes (GpuCount > 0)
			K8sStatus: &readyStatus, // Only query nodes with Ready status
			Limit:     10000,        // Set a sufficiently large limit
		})

	if err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.String("error.message", err.Error()))
		span.SetStatus(codes.Error, err.Error())
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

	span.SetAttributes(
		attribute.Int("nodes.total_count", len(nodes)),
		attribute.Int("nodes.gpu_count", gpuNodeCount),
		attribute.Int("gpu.total_capacity", totalCapacity),
	)
	span.SetStatus(codes.Ok, "")

	log.Infof("Cluster GPU capacity from database: %d GPUs across %d nodes", totalCapacity, gpuNodeCount)
	return totalCapacity, nil
}

// buildPodToWorkloadMapping builds mapping from Pod UID to Workload
func (j *GpuAggregationJob) buildPodToWorkloadMapping(
	ctx context.Context,
	clusterName string,
	dbPods []*dbmodel.GpuPods) (map[string]*dbmodel.GpuWorkload, error) {

	span, ctx := trace.StartSpanFromContext(ctx, "buildPodToWorkloadMapping.query")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("cluster.name", clusterName),
		attribute.Int("pods.input_count", len(dbPods)),
	)

	if len(dbPods) == 0 {
		span.SetStatus(codes.Ok, "No pods to process")
		return make(map[string]*dbmodel.GpuWorkload), nil
	}

	// Collect all Pod UIDs
	podUIDs := make([]string, 0, len(dbPods))
	for _, pod := range dbPods {
		podUIDs = append(podUIDs, pod.UID)
	}

	// Query Pod to Workload reference relationships
	refSpan, refCtx := trace.StartSpanFromContext(ctx, "listWorkloadPodReferences")
	refSpan.SetAttributes(attribute.Int("pod_uids.count", len(podUIDs)))

	workloadRefs, err := database.GetFacadeForCluster(clusterName).GetWorkload().
		ListWorkloadPodReferencesByPodUids(refCtx, podUIDs)
	if err != nil {
		refSpan.RecordError(err)
		refSpan.SetAttributes(attribute.String("error.message", err.Error()))
		refSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(refSpan)

		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to list workload pod references: %w", err)
	}
	refSpan.SetAttributes(attribute.Int("references.count", len(workloadRefs)))
	refSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(refSpan)

	if len(workloadRefs) == 0 {
		log.Infof("No workload references found for pods")
		span.SetAttributes(attribute.Int("references.count", 0))
		span.SetStatus(codes.Ok, "No references found")
		return make(map[string]*dbmodel.GpuWorkload), nil
	}

	// Collect all Workload UIDs
	workloadUIDs := make([]string, 0, len(workloadRefs))
	podToWorkloadUID := make(map[string]string)
	for _, ref := range workloadRefs {
		workloadUIDs = append(workloadUIDs, ref.WorkloadUID)
		podToWorkloadUID[ref.PodUID] = ref.WorkloadUID
	}

	// Query top-level Workload information (including labels)
	workloadSpan, workloadCtx := trace.StartSpanFromContext(ctx, "listTopLevelWorkloads")
	workloadSpan.SetAttributes(attribute.Int("workload_uids.count", len(workloadUIDs)))

	workloads, err := database.GetFacadeForCluster(clusterName).GetWorkload().
		ListTopLevelWorkloadByUids(workloadCtx, workloadUIDs)
	if err != nil {
		workloadSpan.RecordError(err)
		workloadSpan.SetAttributes(attribute.String("error.message", err.Error()))
		workloadSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(workloadSpan)

		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to list top level workloads: %w", err)
	}
	workloadSpan.SetAttributes(attribute.Int("workloads.count", len(workloads)))
	workloadSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(workloadSpan)

	// Build mapping from Workload UID to Workload
	workloadUIDToWorkload := make(map[string]*dbmodel.GpuWorkload)
	for i := range workloads {
		workloadUIDToWorkload[workloads[i].UID] = workloads[i]
	}

	// Build mapping from Pod UID to Workload
	result := make(map[string]*dbmodel.GpuWorkload)
	for podUID, workloadUID := range podToWorkloadUID {
		if workload, exists := workloadUIDToWorkload[workloadUID]; exists {
			result[podUID] = workload
		}
	}

	span.SetAttributes(
		attribute.Int("result.mapping_count", len(result)),
		attribute.Int("result.workload_count", len(workloads)),
	)
	span.SetStatus(codes.Ok, "")

	log.Infof("Built pod to workload mapping: %d pods, %d workloads", len(result), len(workloads))
	return result, nil
}

// loadConfig loads configuration from config manager
func (j *GpuAggregationJob) loadConfig(ctx context.Context) error {
	var cfg model.GpuAggregationConfig

	// Read configuration from config manager
	err := j.configManager.Get(ctx, ConfigKeyGpuAggregation, &cfg)
	if err != nil {
		// Config doesn't exist
		log.Infof("GPU aggregation config not found (key: %s), job will not run. Please set config first.", ConfigKeyGpuAggregation)
		return fmt.Errorf("config not found: %w", err)
	}

	j.config = &cfg
	log.Infof("GPU aggregation config loaded successfully: enabled=%v, sampling_enabled=%v, sampling_interval=%s",
		cfg.Enabled, cfg.Sampling.Enabled, cfg.Sampling.Interval)

	return nil
}

// ReloadConfig reloads configuration (for hot config reload)
func (j *GpuAggregationJob) ReloadConfig(ctx context.Context) error {
	log.Infof("Reloading GPU aggregation config from config manager")
	return j.loadConfig(ctx)
}

// GetConfig gets current configuration (read-only)
func (j *GpuAggregationJob) GetConfig() *model.GpuAggregationConfig {
	return j.config
}

// UpdateConfig updates configuration to config manager
func (j *GpuAggregationJob) UpdateConfig(ctx context.Context, cfg *model.GpuAggregationConfig, updatedBy string) error {
	err := j.configManager.Set(ctx, ConfigKeyGpuAggregation, cfg,
		config.WithDescription("GPU utilization aggregation job configuration"),
		config.WithCategory("job"),
		config.WithUpdatedBy(updatedBy),
		config.WithRecordHistory(true),
		config.WithChangeReason("Update GPU aggregation job config"),
	)
	if err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	// Update local configuration
	j.config = cfg
	log.Infof("GPU aggregation config updated successfully by %s", updatedBy)

	return nil
}

// InitDefaultConfig initializes default configuration (if config doesn't exist)
func InitDefaultConfig(ctx context.Context, clusterName string) error {
	configManager := config.GetConfigManagerForCluster(clusterName)

	// Check if configuration already exists
	exists, err := configManager.Exists(ctx, ConfigKeyGpuAggregation)
	if err != nil {
		return fmt.Errorf("failed to check config existence: %w", err)
	}

	if exists {
		log.Infof("GPU aggregation config already exists for cluster: %s", clusterName)
		return nil
	}

	// Create default configuration
	defaultConfig := &model.GpuAggregationConfig{
		Enabled: true,
	}

	// Sampling configuration
	defaultConfig.Sampling.Enabled = true
	defaultConfig.Sampling.Interval = "5m"
	defaultConfig.Sampling.Timeout = "2m"

	// Aggregation configuration
	defaultConfig.Aggregation.Enabled = true
	defaultConfig.Aggregation.TriggerOffsetMinutes = 5
	defaultConfig.Aggregation.Timeout = "5m"

	// Dimension configuration
	defaultConfig.Dimensions.Cluster.Enabled = true

	defaultConfig.Dimensions.Namespace.Enabled = true
	defaultConfig.Dimensions.Namespace.IncludeSystemNamespaces = false
	defaultConfig.Dimensions.Namespace.ExcludeNamespaces = []string{}

	defaultConfig.Dimensions.Label.Enabled = true
	defaultConfig.Dimensions.Label.LabelKeys = []string{"app", "team", "env"}
	defaultConfig.Dimensions.Label.AnnotationKeys = []string{"project", "cost-center"}
	defaultConfig.Dimensions.Label.DefaultValue = "unknown"

	defaultConfig.Dimensions.Workload.Enabled = true

	// Prometheus configuration
	defaultConfig.Prometheus.WorkloadUtilizationQuery = `avg(dcgm_gpu_utilization{pod_uid="%s"})`
	defaultConfig.Prometheus.QueryStep = 30 // 30 seconds
	defaultConfig.Prometheus.QueryTimeout = "30s"

	// Save default configuration
	err = configManager.Set(ctx, ConfigKeyGpuAggregation, defaultConfig,
		config.WithDescription("GPU utilization aggregation job configuration"),
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
