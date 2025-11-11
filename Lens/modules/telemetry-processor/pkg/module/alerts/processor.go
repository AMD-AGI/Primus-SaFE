package alerts

import (
	"context"
	"encoding/json"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coremodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// ProcessAlertFromLog processes an alert generated from log rules (exported for use by log module)
func ProcessAlertFromLog(ctx context.Context, alert *UnifiedAlert) error {
	return processAlert(ctx, alert)
}

// processAlert is the main entry point for processing an alert
func processAlert(ctx context.Context, alert *UnifiedAlert) error {
	log.GlobalLogger().WithContext(ctx).Infof("Processing alert: %s (source: %s, status: %s)",
		alert.AlertName, alert.Source, alert.Status)

	// Step 1: Check if alert already exists (deduplication)
	existing, err := checkAlertExists(ctx, alert.ID, alert.Status)
	if err != nil {
		return err
	}

	if existing != nil {
		// Update existing alert
		return updateExistingAlert(ctx, existing, alert)
	}

	// Step 2: Enrich alert with additional context
	if err := enrichAlert(ctx, alert); err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to enrich alert %s: %v", alert.ID, err)
		// Continue processing even if enrichment fails
	}

	// Step 3: Check if alert should be silenced
	silenced, err := checkSilenced(ctx, alert)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to check silence for alert %s: %v", alert.ID, err)
	} else if silenced {
		log.GlobalLogger().WithContext(ctx).Infof("Alert %s is silenced", alert.ID)
		alert.Status = StatusSilenced
	}

	// Step 4: Store alert to database
	if err := storeAlert(ctx, alert); err != nil {
		return err
	}

	// Step 5: Update statistics (async)
	go updateAlertStatistics(context.Background(), alert)

	// Step 6: Perform correlation analysis (async)
	go correlateAlerts(context.Background(), alert)

	// Step 7: Route and notify (async, only for firing and not silenced)
	if alert.Status == StatusFiring {
		go routeAndNotify(context.Background(), alert)
	}

	log.GlobalLogger().WithContext(ctx).Infof("Alert %s processed successfully", alert.ID)
	return nil
}

// checkAlertExists checks if an alert with the same ID already exists
func checkAlertExists(ctx context.Context, id string, status string) (*model.AlertEvents, error) {
	facade := database.GetFacade().GetAlert()
	return facade.GetAlertEventsByID(ctx, id)
}

// updateExistingAlert updates an existing alert
func updateExistingAlert(ctx context.Context, existing *model.AlertEvents, newAlert *UnifiedAlert) error {
	log.GlobalLogger().WithContext(ctx).Infof("Updating existing alert: %s", newAlert.ID)

	// Update status and end time
	if newAlert.Status == StatusResolved && existing.Status != StatusResolved {
		endsAt := time.Now()
		if newAlert.EndsAt != nil {
			endsAt = *newAlert.EndsAt
		}

		facade := database.GetFacade().GetAlert()
		if err := facade.UpdateAlertStatus(ctx, newAlert.ID, StatusResolved, &endsAt); err != nil {
			return err
		}

		// Update statistics for resolved alert
		go updateResolvedAlertStatistics(context.Background(), existing, endsAt)

		log.GlobalLogger().WithContext(ctx).Infof("Alert %s resolved", newAlert.ID)
	}

	return nil
}

// enrichAlert enriches alert with additional context information
func enrichAlert(ctx context.Context, alert *UnifiedAlert) error {
	enrichedData := make(map[string]interface{})

	// Enrich with workload information
	if alert.WorkloadID != "" {
		workloadFacade := database.GetFacade().GetWorkload()
		workload, err := workloadFacade.GetGpuWorkloadByUid(ctx, alert.WorkloadID)
		if err == nil && workload != nil {
			enrichedData["workload_name"] = workload.Name
			enrichedData["workload_namespace"] = workload.Namespace
			enrichedData["workload_kind"] = workload.Kind
			enrichedData["workload_status"] = workload.Status

			// Update cluster name if not set
			if alert.ClusterName == "" {
				alert.ClusterName = "default" // You can get this from config
			}
		}
	}

	// Enrich with pod information
	if alert.PodID != "" {
		podFacade := database.GetFacade().GetPod()
		pod, err := podFacade.GetGpuPodsByPodUid(ctx, alert.PodID)
		if err == nil && pod != nil {
			enrichedData["pod_namespace"] = pod.Namespace
			enrichedData["pod_phase"] = pod.Phase
			enrichedData["pod_name"] = pod.Name
			enrichedData["pod_gpu_allocated"] = pod.GpuAllocated

			// Update node name if not set
			if alert.NodeName == "" {
				alert.NodeName = pod.NodeName
			}
		}
	}

	// Enrich with node information
	if alert.NodeName != "" {
		nodeFacade := database.GetFacade().GetNode()
		node, err := nodeFacade.GetNodeByName(ctx, alert.NodeName)
		if err == nil && node != nil {
			enrichedData["node_address"] = node.Address
			enrichedData["node_status"] = node.Status
		}
	}

	alert.EnrichedData = enrichedData
	return nil
}

// checkSilenced checks if an alert matches any active silence rules
func checkSilenced(ctx context.Context, alert *UnifiedAlert) (bool, error) {
	facade := database.GetFacade().GetAlert()
	silences, err := facade.ListActiveSilences(ctx, time.Now(), alert.ClusterName)
	if err != nil {
		return false, err
	}

	for _, silence := range silences {
		if matchSilence(alert, silence) {
			// Record the silenced alert for audit
			recordSilencedAlert(ctx, silence.ID, alert)
			return true, nil
		}
	}

	return false, nil
}

// matchSilence checks if an alert matches a silence rule
func matchSilence(alert *UnifiedAlert, silence *model.AlertSilences) bool {
	// Check if silence is enabled
	if !silence.Enabled {
		return false
	}

	// Check cluster name match (empty silence cluster means all clusters)
	if silence.ClusterName != "" && silence.ClusterName != alert.ClusterName {
		return false
	}

	// Match based on silence type
	switch silence.SilenceType {
	case "label":
		return matchLabelSilence(alert, silence)
	case "alert_name":
		return matchAlertNameSilence(alert, silence)
	case "resource":
		return matchResourceSilence(alert, silence)
	case "expression":
		// TODO: Implement expression-based matching
		return false
	default:
		return false
	}
}

// matchLabelSilence matches alert against label-based silence
func matchLabelSilence(alert *UnifiedAlert, silence *model.AlertSilences) bool {
	var labelMatchers []coremodel.LabelMatcher
	labelMatchersBytes, err := json.Marshal(silence.LabelMatchers)
	if err != nil {
		return false
	}
	if err := json.Unmarshal(labelMatchersBytes, &labelMatchers); err != nil {
		return false
	}

	// All matchers must match
	for _, matcher := range labelMatchers {
		labelValue, exists := alert.Labels[matcher.Name]
		if !exists {
			return false
		}

		switch matcher.Operator {
		case "=":
			if labelValue != matcher.Value {
				return false
			}
		case "!=":
			if labelValue == matcher.Value {
				return false
			}
		case "=~", "!~":
			// TODO: Implement regex matching
			continue
		}
	}

	return len(labelMatchers) > 0
}

// matchAlertNameSilence matches alert against alert name-based silence
func matchAlertNameSilence(alert *UnifiedAlert, silence *model.AlertSilences) bool {
	var alertNames []string
	alertNamesBytes, err := json.Marshal(silence.AlertNames)
	if err != nil {
		return false
	}
	if err := json.Unmarshal(alertNamesBytes, &alertNames); err != nil {
		return false
	}

	for _, name := range alertNames {
		if name == alert.AlertName {
			return true
		}
	}

	return false
}

// matchResourceSilence matches alert against resource-based silence
func matchResourceSilence(alert *UnifiedAlert, silence *model.AlertSilences) bool {
	var resourceFilters []coremodel.ResourceFilter
	resourceFiltersBytes, err := json.Marshal(silence.ResourceFilters)
	if err != nil {
		return false
	}
	if err := json.Unmarshal(resourceFiltersBytes, &resourceFilters); err != nil {
		return false
	}

	// Match any resource filter
	for _, filter := range resourceFilters {
		switch filter.ResourceType {
		case "node":
			if alert.NodeName == filter.ResourceName {
				return true
			}
		case "pod":
			if alert.PodName == filter.ResourceName {
				return true
			}
		case "workload":
			if alert.WorkloadID == filter.ResourceName {
				return true
			}
		}
	}

	return false
}

// matchResourceNames checks if a name matches any in the list
func matchResourceNames(name string, names []string) bool {
	if name == "" {
		return false
	}
	for _, n := range names {
		if n == name {
			return true
		}
		// TODO: Implement pattern matching for wildcards
	}
	return false
}

// matchResourceIDs checks if an ID matches any in the list
func matchResourceIDs(id string, ids []string) bool {
	if id == "" {
		return false
	}
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}

// recordSilencedAlert records a silenced alert for audit trail
func recordSilencedAlert(ctx context.Context, silenceID string, alert *UnifiedAlert) {
	alertDataBytes, _ := json.Marshal(alert)
	var alertDataExt model.ExtType
	json.Unmarshal(alertDataBytes, &alertDataExt)

	silencedAlert := &model.SilencedAlerts{
		SilenceID:   silenceID,
		AlertID:     alert.ID,
		AlertName:   alert.AlertName,
		ClusterName: alert.ClusterName,
		SilencedAt:  time.Now(),
		AlertData:   alertDataExt,
	}

	facade := database.GetFacade().GetAlert()
	if err := facade.CreateSilencedAlerts(ctx, silencedAlert); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to record silenced alert: %v", err)
	}
}

// storeAlert stores the alert to the database
func storeAlert(ctx context.Context, alert *UnifiedAlert) error {
	facade := database.GetFacade().GetAlert()

	// Convert labels and annotations to ExtType
	labelsExt := model.ExtType{}
	for k, v := range alert.Labels {
		labelsExt[k] = v
	}

	annotationsExt := model.ExtType{}
	for k, v := range alert.Annotations {
		annotationsExt[k] = v
	}

	rawDataExt := model.ExtType{}
	if len(alert.RawData) > 0 {
		if err := rawDataExt.Scan(alert.RawData); err != nil {
			return err
		}
	}

	enrichedDataExt := model.ExtType{}
	if alert.EnrichedData != nil {
		for k, v := range alert.EnrichedData {
			enrichedDataExt[k] = v
		}
	}

	alertEvent := &model.AlertEvents{
		ID:           alert.ID,
		Source:       alert.Source,
		AlertName:    alert.AlertName,
		Severity:     alert.Severity,
		Status:       alert.Status,
		StartsAt:     alert.StartsAt,
		Labels:       labelsExt,
		Annotations:  annotationsExt,
		WorkloadID:   alert.WorkloadID,
		PodName:      alert.PodName,
		PodID:        alert.PodID,
		NodeName:     alert.NodeName,
		ClusterName:  alert.ClusterName,
		RawData:      rawDataExt,
		EnrichedData: enrichedDataExt,
	}

	return facade.CreateAlertEvents(ctx, alertEvent)
}

// updateAlertStatistics updates alert statistics in the database
func updateAlertStatistics(ctx context.Context, alert *UnifiedAlert) {
	facade := database.GetFacade().GetAlert()

	// Update hourly statistics
	date := alert.StartsAt.Truncate(24 * time.Hour)
	hour := alert.StartsAt.Hour()

	stat := &model.AlertStatistics{
		Date:        date,
		Hour:        int32(hour),
		AlertName:   alert.AlertName,
		Source:      alert.Source,
		Severity:    alert.Severity,
		WorkloadID:  alert.WorkloadID,
		ClusterName: alert.ClusterName,
		FiringCount: 1,
	}

	if err := facade.CreateOrUpdateAlertStatistics(ctx, stat); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update alert statistics: %v", err)
	}

	// Update daily statistics (hour = 0 means daily aggregate)
	dailyStat := &model.AlertStatistics{
		Date:        date,
		Hour:        0,
		AlertName:   alert.AlertName,
		Source:      alert.Source,
		Severity:    alert.Severity,
		WorkloadID:  alert.WorkloadID,
		ClusterName: alert.ClusterName,
		FiringCount: 1,
	}

	if err := facade.CreateOrUpdateAlertStatistics(ctx, dailyStat); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update daily alert statistics: %v", err)
	}
}

// updateResolvedAlertStatistics updates statistics when an alert is resolved
func updateResolvedAlertStatistics(ctx context.Context, alert *model.AlertEvents, endsAt time.Time) {
	facade := database.GetFacade().GetAlert()

	duration := int64(endsAt.Sub(alert.StartsAt).Seconds())

	// Update hourly statistics
	date := alert.StartsAt.Truncate(24 * time.Hour)
	hour := alert.StartsAt.Hour()

	stat := &model.AlertStatistics{
		Date:                 date,
		Hour:                 int32(hour),
		AlertName:            alert.AlertName,
		Source:               alert.Source,
		Severity:             alert.Severity,
		WorkloadID:           alert.WorkloadID,
		ClusterName:          alert.ClusterName,
		ResolvedCount:        1,
		TotalDurationSeconds: duration,
	}

	if err := facade.CreateOrUpdateAlertStatistics(ctx, stat); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update resolved alert statistics: %v", err)
	}

	// Update daily statistics
	dailyStat := &model.AlertStatistics{
		Date:                 date,
		Hour:                 0,
		AlertName:            alert.AlertName,
		Source:               alert.Source,
		Severity:             alert.Severity,
		WorkloadID:           alert.WorkloadID,
		ClusterName:          alert.ClusterName,
		ResolvedCount:        1,
		TotalDurationSeconds: duration,
	}

	if err := facade.CreateOrUpdateAlertStatistics(ctx, dailyStat); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update daily resolved alert statistics: %v", err)
	}
}
