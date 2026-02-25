// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package distill

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aigateway"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/sql"
)

const (
	topicIntentDistill = "intent.distill"
	topicIntentAudit   = "intent.audit"

	// distillTaskTimeoutSec is the timeout for a distill task (LLM can be slow)
	distillTaskTimeoutSec = 300

	// auditPollInterval is how often we poll for audit results
	auditPollInterval = 3 * time.Second

	// auditPollTimeout is the max time to wait for audit results
	auditPollTimeout = 180 * time.Second
)

// distillRequest is the request payload for the Conductor distill endpoint
type distillRequest struct {
	TargetField string          `json:"target_field"`
	TargetValue string          `json:"target_value"`
	MinSamples  int             `json:"min_samples"`
	Samples     []distillSample `json:"samples"`
}

type distillSample struct {
	WorkloadUID        string                 `json:"workload_uid"`
	Category           string                 `json:"category"`
	IntentDetail       map[string]interface{} `json:"intent_detail"`
	Evidence           map[string]interface{} `json:"evidence"`
	IntentFieldSources map[string]string      `json:"intent_field_sources"`
}

type distillResponse struct {
	TargetField    string          `json:"target_field"`
	TargetValue    string          `json:"target_value"`
	SampleCount    int             `json:"sample_count"`
	CandidateRules []candidateRule `json:"candidate_rules"`
	Reasoning      string          `json:"reasoning"`
}

type candidateRule struct {
	DetectsField      string   `json:"detects_field"`
	DetectsValue      string   `json:"detects_value"`
	Dimension         string   `json:"dimension"`
	Pattern           string   `json:"pattern"`
	Confidence        float64  `json:"confidence"`
	Reasoning         string   `json:"reasoning"`
	DerivedFrom       []string `json:"derived_from"`
	MatchCountInBatch int      `json:"match_count_in_batch"`
}

// TriggerConductorDistillation publishes a distill task to ai-gateway.
// The Conductor bridge agent will pick it up, forward to Conductor,
// and the result is stored asynchronously.
func TriggerConductorDistillation(
	ctx context.Context,
	gwClient *aigateway.Client,
	category string,
	detections []*model.WorkloadDetection,
) {
	if gwClient == nil {
		log.Warn("Flywheel: ai-gateway client not available, skipping distillation")
		return
	}

	// Build samples with enriched evidence from multiple sources
	evidenceFacade := database.NewWorkloadDetectionEvidenceFacade()
	snapshotFacade := database.NewWorkloadCodeSnapshotFacade()
	db := sql.GetDefaultDB()

	samples := make([]distillSample, 0, len(detections))
	for _, det := range detections {
		sample := distillSample{
			WorkloadUID: det.WorkloadUID,
		}
		if det.Category != nil {
			sample.Category = *det.Category
		}
		if det.IntentDetail != nil {
			detailJSON, _ := json.Marshal(det.IntentDetail)
			var detail map[string]interface{}
			if json.Unmarshal(detailJSON, &detail) == nil {
				sample.IntentDetail = detail
			}
		}

		evidence := map[string]interface{}{}
		if det.Framework != "" {
			evidence["framework"] = det.Framework
		}
		if det.WorkloadType != "" {
			evidence["workload_type"] = det.WorkloadType
		}

		// Enrich: cmdline and image from workload_detection_evidence
		evList, _ := evidenceFacade.ListEvidenceByWorkload(ctx, det.WorkloadUID)
		for _, ev := range evList {
			if ev.Evidence == nil {
				continue
			}
			rawJSON, _ := json.Marshal(ev.Evidence)
			var raw map[string]interface{}
			if json.Unmarshal(rawJSON, &raw) != nil {
				continue
			}
			if cmd, ok := raw["cmdline"].(string); ok && cmd != "" {
				evidence["cmdline"] = cmd
			}
			if img, ok := raw["image_name"].(string); ok && img != "" {
				tag, _ := raw["image_tag"].(string)
				if tag != "" {
					evidence["image"] = img + ":" + tag
				} else {
					evidence["image"] = img
				}
			}
		}

		// Enrich: container image from gpu_pods
		if _, hasImg := evidence["image"]; !hasImg && db != nil {
			var image string
			_ = db.WithContext(ctx).
				Raw(`SELECT DISTINCT gp.container_image FROM workload_pod_reference wpr
					JOIN gpu_pods gp ON gp.uid = wpr.pod_uid
					WHERE wpr.workload_uid = ? AND gp.container_image IS NOT NULL LIMIT 1`,
					det.WorkloadUID).Scan(&image).Error
			if image != "" {
				evidence["image"] = image
			}
		}

		// Enrich: pip_freeze from workload_code_snapshot
		snap, _ := snapshotFacade.GetByWorkloadUID(ctx, det.WorkloadUID)
		if snap != nil && snap.PipFreeze != "" {
			const maxPipLen = 2000
			pip := snap.PipFreeze
			if len(pip) > maxPipLen {
				pip = pip[:maxPipLen]
			}
			evidence["pip_freeze"] = pip
		}

		// Enrich: env keys from pod_snapshot
		if db != nil {
			var envJSON string
			_ = db.WithContext(ctx).
				Raw(`SELECT jsonb_agg(e->'name')::text
					FROM workload_pod_reference wpr
					JOIN pod_snapshot ps ON ps.pod_uid = wpr.pod_uid
					CROSS JOIN LATERAL jsonb_array_elements(ps.spec->'containers') c
					CROSS JOIN LATERAL jsonb_array_elements(c->'env') e
					WHERE wpr.workload_uid = ?
					LIMIT 200`, det.WorkloadUID).Scan(&envJSON).Error
			if envJSON != "" && envJSON != "null" {
				var keys []string
				if json.Unmarshal([]byte(envJSON), &keys) == nil && len(keys) > 0 {
					evidence["env_keys"] = keys
				}
			}
		}

		sample.Evidence = evidence

		if det.IntentFieldSources != nil {
			fsJSON, _ := json.Marshal(det.IntentFieldSources)
			var fs map[string]string
			if json.Unmarshal(fsJSON, &fs) == nil && fs != nil {
				sample.IntentFieldSources = fs
			}
		}
		if sample.IntentFieldSources == nil {
			sample.IntentFieldSources = map[string]string{}
		}
		if sample.IntentDetail == nil {
			sample.IntentDetail = map[string]interface{}{}
		}
		if sample.Evidence == nil {
			sample.Evidence = map[string]interface{}{}
		}

		samples = append(samples, sample)
	}

	req := distillRequest{
		TargetField: "category",
		TargetValue: category,
		MinSamples:  5,
		Samples:     samples,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		log.Errorf("Flywheel: failed to marshal distill request: %v", err)
		return
	}

	taskInfo, err := gwClient.Publish(ctx, &aigateway.PublishRequest{
		Topic:      topicIntentDistill,
		Payload:    payload,
		Priority:   5,
		TimeoutSec: distillTaskTimeoutSec,
	})
	if err != nil {
		log.Errorf("Flywheel: failed to publish distill task: %v", err)
		return
	}

	log.Infof("Flywheel: distill task published (id=%s, category=%s, samples=%d)",
		taskInfo.ID, category, len(samples))

	// Poll for result (best-effort; if timeout, rules get stored next cycle)
	pollDistillResult(ctx, gwClient, taskInfo.ID, category)
}

// pollDistillResult polls for the distill task result and stores candidate rules.
func pollDistillResult(ctx context.Context, gwClient *aigateway.Client, taskID string, category string) {
	deadline := time.After(auditPollTimeout)
	ticker := time.NewTicker(auditPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline:
			log.Warnf("Flywheel: distill task %s timed out waiting for result", taskID)
			return
		case <-ticker.C:
			result, err := gwClient.GetResult(ctx, taskID)
			if err != nil {
				log.Warnf("Flywheel: error polling distill task %s: %v", taskID, err)
				return
			}
			if result == nil {
				continue // Still processing
			}

			// Parse response
			var distillResp distillResponse
			if err := json.Unmarshal(result.Result, &distillResp); err != nil {
				log.Errorf("Flywheel: failed to decode distill response for task %s: %v", taskID, err)
				return
			}

			log.Infof("Flywheel: distillation for %s returned %d candidate rules",
				category, len(distillResp.CandidateRules))

			storeCandidateRules(ctx, distillResp.CandidateRules)
			return
		}
	}
}

// storeCandidateRules persists new candidate rules to the intent_rule table
func storeCandidateRules(ctx context.Context, rules []candidateRule) {
	ruleFacade := database.NewIntentRuleFacade()

	for _, cr := range rules {
		exists, err := ruleFacade.ExistsByPatternAndValue(ctx, cr.Pattern, cr.DetectsValue)
		if err != nil {
			log.Errorf("Flywheel: failed to check rule existence: %v", err)
			continue
		}
		if exists {
			log.Infof("Flywheel: skipping duplicate rule (pattern=%q, value=%s)", cr.Pattern, cr.DetectsValue)
			continue
		}

		derivedFromJSON, _ := json.Marshal(cr.DerivedFrom)

		rule := &model.IntentRule{
			DetectsField: cr.DetectsField,
			DetectsValue: cr.DetectsValue,
			Dimension:    cr.Dimension,
			Pattern:      cr.Pattern,
			Confidence:   cr.Confidence,
			Reasoning:    cr.Reasoning,
			DerivedFrom:  derivedFromJSON,
			Status:       "proposed",
		}

		if err := ruleFacade.CreateRule(ctx, rule); err != nil {
			log.Errorf("Flywheel: failed to store candidate rule: %v", err)
			continue
		}

		log.Infof("Flywheel: stored candidate rule %d (%s=%s, pattern=%q)",
			rule.ID, cr.DetectsField, cr.DetectsValue, cr.Pattern)
	}
}

// TriggerConductorAudit publishes an audit task to ai-gateway and polls for results.
func TriggerConductorAudit(
	ctx context.Context,
	gwClient *aigateway.Client,
	samples []AuditSamplePayload,
) (*AuditResponsePayload, error) {
	if gwClient == nil {
		return nil, fmt.Errorf("ai-gateway client not available")
	}

	payload, err := json.Marshal(map[string]interface{}{
		"samples": samples,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal audit request: %w", err)
	}

	taskInfo, err := gwClient.Publish(ctx, &aigateway.PublishRequest{
		Topic:      topicIntentAudit,
		Payload:    payload,
		Priority:   5,
		TimeoutSec: distillTaskTimeoutSec,
	})
	if err != nil {
		return nil, fmt.Errorf("publish audit task: %w", err)
	}

	log.Infof("Flywheel: audit task published (id=%s, samples=%d)", taskInfo.ID, len(samples))

	// Poll for result synchronously (audit needs results for rule promotion)
	deadline := time.After(auditPollTimeout)
	ticker := time.NewTicker(auditPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			return nil, fmt.Errorf("audit task %s timed out", taskInfo.ID)
		case <-ticker.C:
			result, err := gwClient.GetResult(ctx, taskInfo.ID)
			if err != nil {
				return nil, fmt.Errorf("poll audit task %s: %w", taskInfo.ID, err)
			}
			if result == nil {
				continue
			}

			var auditResp AuditResponsePayload
			if err := json.Unmarshal(result.Result, &auditResp); err != nil {
				return nil, fmt.Errorf("decode audit response: %w", err)
			}

			return &auditResp, nil
		}
	}
}

// AuditSamplePayload matches the Conductor AuditSample schema
type AuditSamplePayload struct {
	WorkloadUID        string                 `json:"workload_uid"`
	RuleID             int64                  `json:"rule_id"`
	RulePattern        string                 `json:"rule_pattern"`
	RuleDimension      string                 `json:"rule_dimension"`
	RuleDetectsField   string                 `json:"rule_detects_field"`
	RuleDetectsValue   string                 `json:"rule_detects_value"`
	CurrentCategory    string                 `json:"current_category"`
	CurrentIntentDetail map[string]interface{} `json:"current_intent_detail"`
	Evidence           map[string]interface{} `json:"evidence"`
}

// AuditResponsePayload matches the Conductor AuditResponse schema
type AuditResponsePayload struct {
	TotalAudited      int            `json:"total_audited"`
	ConsistentCount   int            `json:"consistent_count"`
	InconsistentCount int            `json:"inconsistent_count"`
	Verdicts          []AuditVerdict `json:"verdicts"`
}

// AuditVerdict represents a single audit verdict from Conductor
type AuditVerdict struct {
	WorkloadUID       string                 `json:"workload_uid"`
	RuleID            int64                  `json:"rule_id"`
	Consistent        bool                   `json:"consistent"`
	LLMCategory       string                 `json:"llm_category"`
	LLMIntentDetail   map[string]interface{} `json:"llm_intent_detail"`
	DiscrepancyFields []string               `json:"discrepancy_fields"`
	Explanation       string                 `json:"explanation"`
	Confidence        float64                `json:"confidence"`
}
