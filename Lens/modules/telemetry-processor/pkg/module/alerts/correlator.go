package alerts

import (
	"context"
	"encoding/json"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/google/uuid"
)

const (
	// Time window for correlation (5 minutes before and after)
	correlationTimeWindow = 5 * time.Minute

	// Minimum correlation score to create a correlation
	minCorrelationScore = 0.3
)

// correlateAlerts performs correlation analysis for a given alert
func correlateAlerts(ctx context.Context, alert *UnifiedAlert) {
	log.GlobalLogger().WithContext(ctx).Infof("Starting correlation analysis for alert: %s", alert.ID)

	// Find potentially related alerts within time window
	relatedAlerts, err := findRelatedAlerts(ctx, alert)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to find related alerts: %v", err)
		return
	}

	if len(relatedAlerts) == 0 {
		log.GlobalLogger().WithContext(ctx).Infof("No related alerts found for alert: %s", alert.ID)
		return
	}

	// Analyze correlations
	correlations := analyzeCorrelations(alert, relatedAlerts)

	// Store significant correlations
	if len(correlations) > 0 {
		correlationID := uuid.New().String()
		for _, corr := range correlations {
			if err := storeCorrelation(ctx, correlationID, alert.ID, corr); err != nil {
				log.GlobalLogger().WithContext(ctx).Errorf("Failed to store correlation: %v", err)
			}
		}
		log.GlobalLogger().WithContext(ctx).Infof("Stored %d correlations for alert: %s", len(correlations), alert.ID)
	}
}

// findRelatedAlerts finds alerts that might be related to the given alert
func findRelatedAlerts(ctx context.Context, alert *UnifiedAlert) ([]*model.AlertEvents, error) {
	facade := database.GetFacade().GetAlert()

	// Calculate time window
	startTime := alert.StartsAt.Add(-correlationTimeWindow)
	endTime := alert.StartsAt.Add(correlationTimeWindow)

	// Query alerts within time window
	filter := &database.AlertEventsFilter{
		StartsAfter:  &startTime,
		StartsBefore: &endTime,
		Status:       stringPtr(StatusFiring),
		Limit:        100, // Limit to avoid excessive processing
	}

	// Add entity filters for better correlation
	if alert.WorkloadID != "" {
		filter.WorkloadID = &alert.WorkloadID
	} else if alert.PodName != "" {
		filter.PodName = &alert.PodName
	} else if alert.NodeName != "" {
		filter.NodeName = &alert.NodeName
	}

	alerts, _, err := facade.ListAlertEventss(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Filter out the current alert itself
	result := make([]*model.AlertEvents, 0, len(alerts))
	for _, a := range alerts {
		if a.ID != alert.ID {
			result = append(result, a)
		}
	}

	return result, nil
}

// analyzeCorrelations analyzes correlation between alerts
func analyzeCorrelations(alert *UnifiedAlert, relatedAlerts []*model.AlertEvents) []*correlationInfo {
	correlations := make([]*correlationInfo, 0)

	for _, related := range relatedAlerts {
		// Calculate correlation score
		score, corrType, reason := calculateCorrelation(alert, related)

		if score >= minCorrelationScore {
			correlations = append(correlations, &correlationInfo{
				RelatedAlertID:   related.ID,
				CorrelationType:  corrType,
				CorrelationScore: score,
				Reason:           reason,
			})
		}
	}

	return correlations
}

// correlationInfo holds information about a correlation
type correlationInfo struct {
	RelatedAlertID   string
	CorrelationType  string
	CorrelationScore float64
	Reason           string
}

// calculateCorrelation calculates the correlation score between two alerts
func calculateCorrelation(alert *UnifiedAlert, related *model.AlertEvents) (float64, string, string) {
	score := 0.0
	reasons := make([]string, 0)
	corrType := ""

	// Time-based correlation
	timeDiff := alert.StartsAt.Sub(related.StartsAt).Abs()
	if timeDiff < 1*time.Minute {
		score += 0.4
		reasons = append(reasons, "occurred within 1 minute")
		if corrType == "" {
			corrType = CorrelationTypeTime
		}
	} else if timeDiff < 5*time.Minute {
		score += 0.2
		reasons = append(reasons, "occurred within 5 minutes")
		if corrType == "" {
			corrType = CorrelationTypeTime
		}
	}

	// Entity-based correlation
	if alert.WorkloadID != "" && alert.WorkloadID == related.WorkloadID {
		score += 0.5
		reasons = append(reasons, "same workload")
		corrType = CorrelationTypeEntity
	} else if alert.PodName != "" && alert.PodName == related.PodName {
		score += 0.4
		reasons = append(reasons, "same pod")
		corrType = CorrelationTypeEntity
	} else if alert.NodeName != "" && alert.NodeName == related.NodeName {
		score += 0.3
		reasons = append(reasons, "same node")
		corrType = CorrelationTypeEntity
	}

	// Cross-source correlation (metric + log + trace)
	if alert.Source != related.Source {
		score += 0.3
		reasons = append(reasons, "cross-source correlation")
		if corrType != "" {
			corrType = CorrelationTypeCrossSource
		}
	}

	// Causal correlation based on alert types
	causalScore, causalReason := detectCausalRelationship(alert, related)
	if causalScore > 0 {
		score += causalScore
		reasons = append(reasons, causalReason)
		if causalScore > 0.3 {
			corrType = CorrelationTypeCausal
		}
	}

	reason := ""
	if len(reasons) > 0 {
		reason = reasons[0]
		for i := 1; i < len(reasons); i++ {
			reason += "; " + reasons[i]
		}
	}

	return score, corrType, reason
}

// detectCausalRelationship detects if there's a causal relationship between alerts
func detectCausalRelationship(alert *UnifiedAlert, related *model.AlertEvents) (float64, string) {
	// Parse related alert labels
	var relatedLabels map[string]string
	labelsBytes, err := json.Marshal(related.Labels)
	if err != nil {
		return 0, ""
	}
	if err := json.Unmarshal(labelsBytes, &relatedLabels); err != nil {
		return 0, ""
	}

	relatedAlertName := related.AlertName

	// Define known causal relationships
	causalRules := []struct {
		cause  string
		effect string
		score  float64
		reason string
	}{
		{"GPUMemoryHigh", "OOMError", 0.8, "high GPU memory leads to OOM"},
		{"NetworkLatencyHigh", "TrainingSlowdown", 0.7, "network latency causes training slowdown"},
		{"NodeDown", "PodRestart", 0.9, "node failure causes pod restart"},
		{"DiskSpaceLow", "CheckpointFailed", 0.8, "low disk space causes checkpoint failure"},
		{"NCCLError", "TrainingHang", 0.7, "NCCL error causes training hang"},
		{"HighTemperature", "GPUThrottling", 0.8, "high temperature causes GPU throttling"},
	}

	// Check if current alert is caused by related alert
	for _, rule := range causalRules {
		if relatedAlertName == rule.cause && alert.AlertName == rule.effect {
			return rule.score, rule.reason
		}
	}

	// Check reverse relationship
	for _, rule := range causalRules {
		if alert.AlertName == rule.cause && relatedAlertName == rule.effect {
			return rule.score, rule.reason
		}
	}

	return 0, ""
}

// storeCorrelation stores a correlation relationship in the database
func storeCorrelation(ctx context.Context, correlationID, alertID string, corr *correlationInfo) error {
	facade := database.GetFacade().GetAlert()

	metadata := model.ExtType{
		"reason": corr.Reason,
	}

	correlation := &model.AlertCorrelations{
		CorrelationID:       correlationID,
		AlertID:             corr.RelatedAlertID,
		CorrelationType:     corr.CorrelationType,
		CorrelationScore:    corr.CorrelationScore,
		CorrelationReason:   corr.Reason,
		CorrelationMetadata: metadata,
	}

	return facade.CreateAlertCorrelations(ctx, correlation)
}

// GetAlertCorrelations retrieves correlations for a given alert
func GetAlertCorrelations(ctx context.Context, alertID string) ([]*AlertCorrelationResponse, error) {
	facade := database.GetFacade().GetAlert()

	// Get all correlations for this alert
	correlations, err := facade.ListAlertCorrelationssByAlertID(ctx, alertID)
	if err != nil {
		return nil, err
	}

	if len(correlations) == 0 {
		return []*AlertCorrelationResponse{}, nil
	}

	// Group correlations by correlation ID
	corrGroups := make(map[string][]*model.AlertCorrelations)
	for _, corr := range correlations {
		corrGroups[corr.CorrelationID] = append(corrGroups[corr.CorrelationID], corr)
	}

	// Build response
	responses := make([]*AlertCorrelationResponse, 0, len(corrGroups))
	for corrID, group := range corrGroups {
		// Get all alert IDs in this correlation group
		alertIDs := make([]string, 0, len(group)+1)
		alertIDs = append(alertIDs, alertID)
		for _, corr := range group {
			alertIDs = append(alertIDs, corr.AlertID)
		}

		// Fetch all alerts
		alerts := make([]*UnifiedAlert, 0, len(alertIDs))
		for _, id := range alertIDs {
			alert, err := facade.GetAlertEventsByID(ctx, id)
			if err != nil || alert == nil {
				continue
			}

			unified := convertAlertEventToUnified(alert)
			alerts = append(alerts, unified)
		}

		// Use the first correlation's type and score
		var corrType string
		var corrScore float64
		var reason string
		if len(group) > 0 {
			corrType = group[0].CorrelationType
			corrScore = group[0].CorrelationScore
			reason = group[0].CorrelationReason
		}

		responses = append(responses, &AlertCorrelationResponse{
			CorrelationID:    corrID,
			Alerts:           alerts,
			CorrelationType:  corrType,
			CorrelationScore: corrScore,
			Reason:           reason,
		})
	}

	return responses, nil
}

// convertAlertEventToUnified converts database model to unified alert
func convertAlertEventToUnified(event *model.AlertEvents) *UnifiedAlert {
	labels := make(map[string]string)
	if event.Labels != nil {
		labelsBytes, _ := json.Marshal(event.Labels)
		json.Unmarshal(labelsBytes, &labels)
	}

	annotations := make(map[string]string)
	if event.Annotations != nil {
		annotationsBytes, _ := json.Marshal(event.Annotations)
		json.Unmarshal(annotationsBytes, &annotations)
	}

	enrichedData := make(map[string]interface{})
	if event.EnrichedData != nil {
		enrichedData = event.EnrichedData
	}

	var rawData []byte
	if event.RawData != nil {
		rawDataBytes, _ := json.Marshal(event.RawData)
		rawData = rawDataBytes
	}

	var endsAt *time.Time
	if !event.EndsAt.IsZero() {
		endsAt = &event.EndsAt
	}

	return &UnifiedAlert{
		ID:           event.ID,
		Source:       event.Source,
		AlertName:    event.AlertName,
		Severity:     event.Severity,
		Status:       event.Status,
		StartsAt:     event.StartsAt,
		EndsAt:       endsAt,
		Labels:       labels,
		Annotations:  annotations,
		WorkloadID:   event.WorkloadID,
		PodName:      event.PodName,
		PodID:        event.PodID,
		NodeName:     event.NodeName,
		ClusterName:  event.ClusterName,
		RawData:      rawData,
		EnrichedData: enrichedData,
	}
}

// stringPtr returns a pointer to a string
func stringPtr(s string) *string {
	return &s
}
