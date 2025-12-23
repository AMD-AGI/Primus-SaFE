package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// ======================== Detection Status Response Models ========================

// DetectionStatusResponse represents the full detection status for a workload
type DetectionStatusResponse struct {
	WorkloadUID      string                  `json:"workload_uid"`
	Status           string                  `json:"status"`                      // Detection status: unknown, suspected, confirmed, verified, conflict
	DetectionState   string                  `json:"detection_state"`             // Active detection state: pending, in_progress, completed, failed
	Framework        string                  `json:"framework,omitempty"`         // Primary detected framework
	Frameworks       []string                `json:"frameworks,omitempty"`        // All detected frameworks
	WorkloadType     string                  `json:"workload_type,omitempty"`     // training or inference
	Confidence       float64                 `json:"confidence"`                  // Aggregated confidence [0-1]
	FrameworkLayer   string                  `json:"framework_layer,omitempty"`   // wrapper or base
	WrapperFramework string                  `json:"wrapper_framework,omitempty"` // Wrapper framework name
	BaseFramework    string                  `json:"base_framework,omitempty"`    // Base framework name
	EvidenceCount    int32                   `json:"evidence_count"`              // Total evidence records
	EvidenceSources  []string                `json:"evidence_sources"`            // Sources that contributed evidence
	AttemptCount     int32                   `json:"attempt_count"`               // Detection attempts made
	MaxAttempts      int32                   `json:"max_attempts"`                // Max detection attempts
	LastAttemptAt    *time.Time              `json:"last_attempt_at,omitempty"`   // Last detection attempt time
	NextAttemptAt    *time.Time              `json:"next_attempt_at,omitempty"`   // Next scheduled attempt time
	ConfirmedAt      *time.Time              `json:"confirmed_at,omitempty"`      // When detection was confirmed
	CreatedAt        time.Time               `json:"created_at"`                  // Detection record creation time
	UpdatedAt        time.Time               `json:"updated_at"`                  // Last update time
	Coverage         []DetectionCoverageItem `json:"coverage"`                    // Coverage status for each source
	Tasks            []DetectionTaskItem     `json:"tasks"`                       // Related detection tasks
	HasConflicts     bool                    `json:"has_conflicts"`               // Whether conflicts exist
	Conflicts        []interface{}           `json:"conflicts,omitempty"`         // Conflict details if any
}

// DetectionCoverageItem represents coverage status for a single source
type DetectionCoverageItem struct {
	Source           string     `json:"source"`                       // process, log, image, label, wandb, import
	Status           string     `json:"status"`                       // pending, collecting, collected, failed, not_applicable
	AttemptCount     int32      `json:"attempt_count"`                // Collection attempts
	LastAttemptAt    *time.Time `json:"last_attempt_at,omitempty"`    // Last collection attempt
	LastSuccessAt    *time.Time `json:"last_success_at,omitempty"`    // Last successful collection
	LastError        string     `json:"last_error,omitempty"`         // Last error if any
	EvidenceCount    int32      `json:"evidence_count"`               // Evidence records from this source
	CoveredFrom      *time.Time `json:"covered_from,omitempty"`       // Log scan start time (log source only)
	CoveredTo        *time.Time `json:"covered_to,omitempty"`         // Log scan end time (log source only)
	LogAvailableFrom *time.Time `json:"log_available_from,omitempty"` // Earliest available log time
	LogAvailableTo   *time.Time `json:"log_available_to,omitempty"`   // Latest available log time
	HasGap           bool       `json:"has_gap,omitempty"`            // Whether there's uncovered log window
}

// DetectionTaskItem represents a detection-related task
type DetectionTaskItem struct {
	TaskType         string                 `json:"task_type"`                   // Task type
	Status           string                 `json:"status"`                      // Task status
	LockOwner        string                 `json:"lock_owner,omitempty"`        // Current lock owner
	CreatedAt        time.Time              `json:"created_at"`                  // Task creation time
	UpdatedAt        time.Time              `json:"updated_at"`                  // Last update time
	AttemptCount     int                    `json:"attempt_count,omitempty"`     // Attempt count from ext
	NextAttemptAt    *time.Time             `json:"next_attempt_at,omitempty"`   // Next attempt time from ext
	CoordinatorState string                 `json:"coordinator_state,omitempty"` // State for coordinator tasks
	Ext              map[string]interface{} `json:"ext,omitempty"`               // Additional task data
}

// DetectionSummaryResponse represents a summary of all detections
type DetectionSummaryResponse struct {
	TotalWorkloads       int64                     `json:"total_workloads"`
	StatusCounts         map[string]int64          `json:"status_counts"`          // Count by detection status
	DetectionStateCounts map[string]int64          `json:"detection_state_counts"` // Count by detection state
	RecentDetections     []DetectionStatusResponse `json:"recent_detections"`      // Recently updated detections
}

// ======================== API Handlers ========================

// GetDetectionStatus retrieves the full detection status for a workload
// GET /detection-status/:workload_uid
func GetDetectionStatus(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get cluster clients: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get cluster", err))
		return
	}

	workloadUID := ctx.Param("workload_uid")
	if workloadUID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "workload_uid is required", nil))
		return
	}

	facade := database.GetFacadeForCluster(clients.ClusterName)

	// Get detection record
	detection, err := facade.GetWorkloadDetection().GetDetection(ctx.Request.Context(), workloadUID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get detection: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get detection", err))
		return
	}

	if detection == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "detection not found", nil))
		return
	}

	// Get coverage records
	coverages, err := facade.GetDetectionCoverage().ListCoverageByWorkload(ctx.Request.Context(), workloadUID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to get coverage records: %v", err)
		coverages = []*model.DetectionCoverage{}
	}

	// Get related tasks
	taskFacade := database.NewWorkloadTaskFacade()
	tasks, err := taskFacade.ListTasksByWorkload(ctx.Request.Context(), workloadUID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to get tasks: %v", err)
		tasks = []*model.WorkloadTaskState{}
	}

	// Filter to detection-related tasks
	detectionTasks := filterDetectionTasks(tasks)

	// Build response
	response := buildDetectionStatusResponse(detection, coverages, detectionTasks)

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), response))
}

// ListDetectionStatuses lists detection statuses with filtering
// GET /detection-status
func ListDetectionStatuses(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get cluster clients: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get cluster", err))
		return
	}

	// Parse query parameters
	status := ctx.Query("status")        // Filter by detection status
	detectionState := ctx.Query("state") // Filter by detection state
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	facade := database.GetFacadeForCluster(clients.ClusterName)
	detectionFacade := facade.GetWorkloadDetection()

	var detections []*model.WorkloadDetection
	var total int64

	// Query based on filters
	if status != "" {
		detections, total, err = detectionFacade.ListDetectionsByStatus(ctx.Request.Context(), status, pageSize, offset)
	} else if detectionState != "" {
		allDetections, err2 := detectionFacade.ListDetectionsByDetectionState(ctx.Request.Context(), detectionState)
		if err2 != nil {
			err = err2
		} else {
			total = int64(len(allDetections))
			// Manual pagination
			start := offset
			end := offset + pageSize
			if start > len(allDetections) {
				start = len(allDetections)
			}
			if end > len(allDetections) {
				end = len(allDetections)
			}
			detections = allDetections[start:end]
		}
	} else {
		// List all with pagination (by updated_at desc)
		detections, total, err = detectionFacade.ListDetectionsByStatus(ctx.Request.Context(), "", pageSize, offset)
		if err != nil {
			// Fallback: query without status filter
			db := facade.GetSystemConfig().GetDB()
			err = db.WithContext(ctx.Request.Context()).
				Table(model.TableNameWorkloadDetection).
				Count(&total).Error
			if err == nil {
				err = db.WithContext(ctx.Request.Context()).
					Table(model.TableNameWorkloadDetection).
					Order("updated_at DESC").
					Limit(pageSize).
					Offset(offset).
					Find(&detections).Error
			}
		}
	}

	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list detections: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list detections", err))
		return
	}

	// Build response (simplified, without coverage/tasks for list view)
	responses := make([]DetectionStatusResponse, 0, len(detections))
	for _, d := range detections {
		responses = append(responses, buildDetectionStatusResponse(d, nil, nil))
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"data":      responses,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}))
}

// GetDetectionSummary returns a summary of all detection statuses
// GET /detection-status/summary
func GetDetectionSummary(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get cluster clients: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get cluster", err))
		return
	}

	db := database.GetFacadeForCluster(clients.ClusterName).GetSystemConfig().GetDB()

	// Count total workloads with detection
	var totalWorkloads int64
	if err := db.WithContext(ctx.Request.Context()).
		Table(model.TableNameWorkloadDetection).
		Count(&totalWorkloads).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to count detections: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to count detections", err))
		return
	}

	// Count by status
	type StatusCount struct {
		Status string `gorm:"column:status"`
		Count  int64  `gorm:"column:count"`
	}
	var statusCounts []StatusCount
	if err := db.WithContext(ctx.Request.Context()).
		Table(model.TableNameWorkloadDetection).
		Select("status, COUNT(*) as count").
		Group("status").
		Scan(&statusCounts).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to count by status: %v", err)
	}

	statusCountMap := make(map[string]int64)
	for _, sc := range statusCounts {
		statusCountMap[sc.Status] = sc.Count
	}

	// Count by detection state
	var stateCounts []StatusCount
	if err := db.WithContext(ctx.Request.Context()).
		Table(model.TableNameWorkloadDetection).
		Select("detection_state as status, COUNT(*) as count").
		Group("detection_state").
		Scan(&stateCounts).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to count by state: %v", err)
	}

	stateCountMap := make(map[string]int64)
	for _, sc := range stateCounts {
		stateCountMap[sc.Status] = sc.Count
	}

	// Get recent detections (last 10 updated)
	var recentDetections []*model.WorkloadDetection
	if err := db.WithContext(ctx.Request.Context()).
		Table(model.TableNameWorkloadDetection).
		Order("updated_at DESC").
		Limit(10).
		Find(&recentDetections).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to get recent detections: %v", err)
	}

	recentResponses := make([]DetectionStatusResponse, 0, len(recentDetections))
	for _, d := range recentDetections {
		recentResponses = append(recentResponses, buildDetectionStatusResponse(d, nil, nil))
	}

	response := DetectionSummaryResponse{
		TotalWorkloads:       totalWorkloads,
		StatusCounts:         statusCountMap,
		DetectionStateCounts: stateCountMap,
		RecentDetections:     recentResponses,
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), response))
}

// GetDetectionCoverage returns the detection coverage for a workload
// GET /detection-status/:workload_uid/coverage
func GetDetectionCoverage(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get cluster clients: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get cluster", err))
		return
	}

	workloadUID := ctx.Param("workload_uid")
	if workloadUID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "workload_uid is required", nil))
		return
	}

	coverages, err := database.GetFacadeForCluster(clients.ClusterName).GetDetectionCoverage().ListCoverageByWorkload(ctx.Request.Context(), workloadUID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get coverage: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get coverage", err))
		return
	}

	items := make([]DetectionCoverageItem, 0, len(coverages))
	for _, c := range coverages {
		items = append(items, buildCoverageItem(c))
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"workload_uid": workloadUID,
		"coverage":     items,
		"total":        len(items),
	}))
}

// GetDetectionTasks returns the detection-related tasks for a workload
// GET /detection-status/:workload_uid/tasks
func GetDetectionTasks(ctx *gin.Context) {
	workloadUID := ctx.Param("workload_uid")
	if workloadUID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "workload_uid is required", nil))
		return
	}

	taskFacade := database.NewWorkloadTaskFacade()
	tasks, err := taskFacade.ListTasksByWorkload(ctx.Request.Context(), workloadUID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get tasks: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get tasks", err))
		return
	}

	// Filter to detection-related tasks
	detectionTasks := filterDetectionTasks(tasks)

	items := make([]DetectionTaskItem, 0, len(detectionTasks))
	for _, t := range detectionTasks {
		items = append(items, buildTaskItem(t))
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"workload_uid": workloadUID,
		"tasks":        items,
		"total":        len(items),
	}))
}

// ======================== Helper Functions ========================

// buildDetectionStatusResponse builds the full detection status response
func buildDetectionStatusResponse(detection *model.WorkloadDetection, coverages []*model.DetectionCoverage, tasks []*model.WorkloadTaskState) DetectionStatusResponse {
	response := DetectionStatusResponse{
		WorkloadUID:      detection.WorkloadUID,
		Status:           detection.Status,
		DetectionState:   detection.DetectionState,
		Framework:        detection.Framework,
		WorkloadType:     detection.WorkloadType,
		Confidence:       detection.Confidence,
		FrameworkLayer:   detection.FrameworkLayer,
		WrapperFramework: detection.WrapperFramework,
		BaseFramework:    detection.BaseFramework,
		EvidenceCount:    detection.EvidenceCount,
		AttemptCount:     detection.AttemptCount,
		MaxAttempts:      detection.MaxAttempts,
		CreatedAt:        detection.CreatedAt,
		UpdatedAt:        detection.UpdatedAt,
		Coverage:         []DetectionCoverageItem{},
		Tasks:            []DetectionTaskItem{},
	}

	// Parse frameworks from JSON
	if len(detection.Frameworks) > 0 {
		var frameworks []string
		if err := detection.Frameworks.UnmarshalTo(&frameworks); err == nil {
			response.Frameworks = frameworks
		}
	}

	// Parse evidence sources from JSON
	if len(detection.EvidenceSources) > 0 {
		var sources []string
		if err := detection.EvidenceSources.UnmarshalTo(&sources); err == nil {
			response.EvidenceSources = sources
		}
	}

	// Parse conflicts
	if len(detection.Conflicts) > 0 {
		var conflicts []interface{}
		if err := detection.Conflicts.UnmarshalTo(&conflicts); err == nil {
			response.Conflicts = conflicts
			response.HasConflicts = len(conflicts) > 0
		}
	}

	// Set optional time fields
	if !detection.LastAttemptAt.IsZero() {
		response.LastAttemptAt = &detection.LastAttemptAt
	}
	if !detection.NextAttemptAt.IsZero() {
		response.NextAttemptAt = &detection.NextAttemptAt
	}
	if !detection.ConfirmedAt.IsZero() {
		response.ConfirmedAt = &detection.ConfirmedAt
	}

	// Build coverage items
	for _, c := range coverages {
		response.Coverage = append(response.Coverage, buildCoverageItem(c))
	}

	// Build task items
	for _, t := range tasks {
		response.Tasks = append(response.Tasks, buildTaskItem(t))
	}

	return response
}

// buildCoverageItem builds a coverage item from model
func buildCoverageItem(coverage *model.DetectionCoverage) DetectionCoverageItem {
	item := DetectionCoverageItem{
		Source:        coverage.Source,
		Status:        coverage.Status,
		AttemptCount:  coverage.AttemptCount,
		LastError:     coverage.LastError,
		EvidenceCount: coverage.EvidenceCount,
	}

	if !coverage.LastAttemptAt.IsZero() {
		item.LastAttemptAt = &coverage.LastAttemptAt
	}
	if !coverage.LastSuccessAt.IsZero() {
		item.LastSuccessAt = &coverage.LastSuccessAt
	}

	// Log source specific fields
	if coverage.Source == "log" {
		if !coverage.CoveredFrom.IsZero() {
			item.CoveredFrom = &coverage.CoveredFrom
		}
		if !coverage.CoveredTo.IsZero() {
			item.CoveredTo = &coverage.CoveredTo
		}
		if !coverage.LogAvailableFrom.IsZero() {
			item.LogAvailableFrom = &coverage.LogAvailableFrom
		}
		if !coverage.LogAvailableTo.IsZero() {
			item.LogAvailableTo = &coverage.LogAvailableTo
		}

		// Check for gap
		if !coverage.LogAvailableTo.IsZero() {
			if coverage.CoveredTo.IsZero() {
				item.HasGap = true
			} else if coverage.LogAvailableTo.After(coverage.CoveredTo) {
				item.HasGap = true
			}
		}
	}

	return item
}

// buildTaskItem builds a task item from model
func buildTaskItem(task *model.WorkloadTaskState) DetectionTaskItem {
	item := DetectionTaskItem{
		TaskType:  task.TaskType,
		Status:    task.Status,
		LockOwner: task.LockOwner,
		CreatedAt: task.CreatedAt,
		UpdatedAt: task.UpdatedAt,
	}

	// Extract useful fields from Ext
	if task.Ext != nil {
		// Attempt count
		if attemptCount, ok := task.Ext["attempt_count"]; ok {
			if count, ok := attemptCount.(float64); ok {
				item.AttemptCount = int(count)
			}
		}

		// Next attempt time
		if nextAttemptStr, ok := task.Ext["next_attempt_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339, nextAttemptStr); err == nil {
				item.NextAttemptAt = &t
			}
		}

		// Coordinator state
		if state, ok := task.Ext["coordinator_state"].(string); ok {
			item.CoordinatorState = state
		}

		// Include full ext for debugging (optional, can be removed for production)
		item.Ext = task.Ext
	}

	return item
}

// filterDetectionTasks filters tasks to only detection-related ones
func filterDetectionTasks(tasks []*model.WorkloadTaskState) []*model.WorkloadTaskState {
	detectionTaskTypes := map[string]bool{
		"detection_coordinator": true,
		"active_detection":      true,
		"process_probe":         true,
		"log_detection":         true,
		"image_probe":           true,
		"label_probe":           true,
	}

	result := make([]*model.WorkloadTaskState, 0)
	for _, t := range tasks {
		if detectionTaskTypes[t.TaskType] {
			result = append(result, t)
		}
	}
	return result
}

// ======================== Evidence API ========================

// DetectionEvidenceItem represents a single evidence record
type DetectionEvidenceItem struct {
	ID               int64                  `json:"id"`
	WorkloadUID      string                 `json:"workload_uid"`
	Source           string                 `json:"source"`            // process, log, image, label, etc.
	SourceType       string                 `json:"source_type"`       // passive or active
	Framework        string                 `json:"framework"`         // Detected framework
	WorkloadType     string                 `json:"workload_type"`     // training or inference
	Confidence       float64                `json:"confidence"`        // Confidence [0-1]
	FrameworkLayer   string                 `json:"framework_layer"`   // wrapper or base
	WrapperFramework string                 `json:"wrapper_framework"` // Wrapper framework
	BaseFramework    string                 `json:"base_framework"`    // Base framework
	Evidence         map[string]interface{} `json:"evidence"`          // Evidence details
	DetectedAt       time.Time              `json:"detected_at"`       // When evidence was collected
	CreatedAt        time.Time              `json:"created_at"`        // Record creation time
}

// GetDetectionEvidence returns evidence records for a workload
// GET /detection-status/:workload_uid/evidence
func GetDetectionEvidence(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get cluster clients: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get cluster", err))
		return
	}

	workloadUID := ctx.Param("workload_uid")
	if workloadUID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "workload_uid is required", nil))
		return
	}

	// Parse optional source filter
	source := ctx.Query("source")

	db := database.GetFacadeForCluster(clients.ClusterName).GetSystemConfig().GetDB()

	var evidenceRecords []*model.WorkloadDetectionEvidence
	query := db.WithContext(ctx.Request.Context()).
		Table(model.TableNameWorkloadDetectionEvidence).
		Where("workload_uid = ?", workloadUID).
		Order("detected_at DESC")

	if source != "" {
		query = query.Where("source = ?", source)
	}

	if err := query.Find(&evidenceRecords).Error; err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get evidence: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get evidence", err))
		return
	}

	items := make([]DetectionEvidenceItem, 0, len(evidenceRecords))
	for _, e := range evidenceRecords {
		items = append(items, DetectionEvidenceItem{
			ID:               e.ID,
			WorkloadUID:      e.WorkloadUID,
			Source:           e.Source,
			SourceType:       e.SourceType,
			Framework:        e.Framework,
			WorkloadType:     e.WorkloadType,
			Confidence:       e.Confidence,
			FrameworkLayer:   e.FrameworkLayer,
			WrapperFramework: e.WrapperFramework,
			BaseFramework:    e.BaseFramework,
			Evidence:         e.Evidence,
			DetectedAt:       e.DetectedAt,
			CreatedAt:        e.CreatedAt,
		})
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"workload_uid": workloadUID,
		"evidence":     items,
		"total":        len(items),
	}))
}

// InitializeDetectionCoverage initializes detection coverage for a workload
// POST /detection-status/:workload_uid/coverage/initialize
func InitializeDetectionCoverage(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get cluster clients: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get cluster", err))
		return
	}

	workloadUID := ctx.Param("workload_uid")
	if workloadUID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "workload_uid is required", nil))
		return
	}

	coverageFacade := database.GetFacadeForCluster(clients.ClusterName).GetDetectionCoverage()

	// Check if already initialized
	existing, err := coverageFacade.ListCoverageByWorkload(ctx.Request.Context(), workloadUID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to check existing coverage: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to check coverage", err))
		return
	}

	if len(existing) > 0 {
		ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
			"message": "coverage already initialized",
			"count":   len(existing),
		}))
		return
	}

	// Initialize coverage
	if err := coverageFacade.InitializeCoverageForWorkload(ctx.Request.Context(), workloadUID); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to initialize coverage: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to initialize coverage", err))
		return
	}

	// Get created records
	coverages, _ := coverageFacade.ListCoverageByWorkload(ctx.Request.Context(), workloadUID)

	items := make([]DetectionCoverageItem, 0, len(coverages))
	for _, c := range coverages {
		items = append(items, buildCoverageItem(c))
	}

	ctx.JSON(http.StatusCreated, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message":  "coverage initialized",
		"coverage": items,
		"count":    len(items),
	}))
}

// ReportLogDetection receives log detection report from telemetry-processor
// POST /detection-status/log-report
func ReportLogDetection(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get cluster clients: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get cluster", err))
		return
	}

	type LogReportRequest struct {
		WorkloadUID    string    `json:"workload_uid" binding:"required"`
		DetectedAt     time.Time `json:"detected_at"`
		LogTimestamp   time.Time `json:"log_timestamp" binding:"required"`
		Framework      string    `json:"framework"`
		Confidence     float64   `json:"confidence"`
		PatternMatched string    `json:"pattern_matched"`
	}

	var req LogReportRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid request body", err))
		return
	}

	facade := database.GetFacadeForCluster(clients.ClusterName)
	coverageFacade := facade.GetDetectionCoverage()

	// Update log available time
	if err := coverageFacade.UpdateLogAvailableTime(ctx.Request.Context(), req.WorkloadUID, req.LogTimestamp); err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to update log available time: %v", err)
	}

	// If framework detected, store evidence
	if req.Framework != "" {
		db := facade.GetSystemConfig().GetDB()
		detectedAt := req.DetectedAt
		if detectedAt.IsZero() {
			detectedAt = time.Now()
		}

		evidence := &model.WorkloadDetectionEvidence{
			WorkloadUID:  req.WorkloadUID,
			Source:       "log",
			SourceType:   "passive",
			Framework:    req.Framework,
			WorkloadType: "training", // Default, can be inferred from pattern
			Confidence:   req.Confidence,
			DetectedAt:   detectedAt,
			CreatedAt:    time.Now(),
			Evidence: model.ExtType{
				"pattern_matched": req.PatternMatched,
				"log_timestamp":   req.LogTimestamp,
			},
		}

		if err := db.WithContext(ctx.Request.Context()).Create(evidence).Error; err != nil {
			log.GlobalLogger().WithContext(ctx).Warningf("Failed to store log evidence: %v", err)
		} else {
			// Increment evidence count
			_ = coverageFacade.IncrementEvidenceCount(ctx.Request.Context(), req.WorkloadUID, "log", 1)
		}
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"status": "ok",
	}))
}

// GetUncoveredLogWindow returns any uncovered log time window
// GET /detection-status/:workload_uid/coverage/log-gap
func GetUncoveredLogWindow(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get cluster clients: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get cluster", err))
		return
	}

	workloadUID := ctx.Param("workload_uid")
	if workloadUID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "workload_uid is required", nil))
		return
	}

	coverageFacade := database.GetFacadeForCluster(clients.ClusterName).GetDetectionCoverage()

	from, to, err := coverageFacade.GetUncoveredLogWindow(ctx.Request.Context(), workloadUID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get uncovered log window: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get log window", err))
		return
	}

	response := gin.H{
		"workload_uid": workloadUID,
		"has_gap":      from != nil && to != nil,
	}

	if from != nil && to != nil {
		response["gap_from"] = from
		response["gap_to"] = to
		response["gap_duration_seconds"] = to.Sub(*from).Seconds()
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), response))
}

// TriggerDetection manually triggers detection for a workload
// POST /detection-status/:workload_uid/trigger
func TriggerDetection(ctx *gin.Context) {
	workloadUID := ctx.Param("workload_uid")
	if workloadUID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "workload_uid is required", nil))
		return
	}

	taskFacade := database.NewWorkloadTaskFacade()

	// Create or update detection coordinator task
	task := &model.WorkloadTaskState{
		WorkloadUID: workloadUID,
		TaskType:    "detection_coordinator",
		Status:      "pending",
		Ext: model.ExtType{
			"triggered_by":           "manual_api",
			"triggered_at":           time.Now().Format(time.RFC3339),
			"initial_delay_seconds":  0, // No delay for manual trigger
			"retry_interval_seconds": 30,
		},
	}

	if err := taskFacade.UpsertTask(ctx.Request.Context(), task); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to create detection task: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to trigger detection", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Detection triggered for workload %s", workloadUID)

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message":      fmt.Sprintf("detection triggered for workload %s", workloadUID),
		"workload_uid": workloadUID,
		"task_type":    "detection_coordinator",
		"status":       "pending",
	}))
}
