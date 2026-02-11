// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package distill

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// distillRequest is the request payload for the Conductor distill endpoint
type distillRequest struct {
	TargetField string            `json:"target_field"`
	TargetValue string            `json:"target_value"`
	MinSamples  int               `json:"min_samples"`
	Samples     []distillSample   `json:"samples"`
}

type distillSample struct {
	WorkloadUID      string                 `json:"workload_uid"`
	Category         string                 `json:"category"`
	IntentDetail     map[string]interface{} `json:"intent_detail"`
	Evidence         map[string]interface{} `json:"evidence"`
	IntentFieldSources map[string]string    `json:"intent_field_sources"`
}

type distillResponse struct {
	TargetField    string           `json:"target_field"`
	TargetValue    string           `json:"target_value"`
	SampleCount    int              `json:"sample_count"`
	CandidateRules []candidateRule  `json:"candidate_rules"`
	Reasoning      string           `json:"reasoning"`
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

var httpClient = &http.Client{
	Timeout: 120 * time.Second,
}

// TriggerConductorDistillation sends a batch of same-category detections
// to Conductor for LLM distillation and stores the resulting candidate rules.
func TriggerConductorDistillation(
	ctx context.Context,
	conductorURL string,
	category string,
	detections []*model.WorkloadDetection,
) {
	// Build samples
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
		// Build evidence from available detection fields
		evidence := map[string]interface{}{}
		if det.Framework != "" {
			evidence["framework"] = det.Framework
		}
		if det.WorkloadType != "" {
			evidence["workload_type"] = det.WorkloadType
		}
		sample.Evidence = evidence

		samples = append(samples, sample)
	}

	req := distillRequest{
		TargetField: "category",
		TargetValue: category,
		MinSamples:  5,
		Samples:     samples,
	}

	body, err := json.Marshal(req)
	if err != nil {
		log.Errorf("Flywheel: failed to marshal distill request: %v", err)
		return
	}

	url := fmt.Sprintf("%s/api/v1/intent/distill", conductorURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Errorf("Flywheel: failed to create distill request: %v", err)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		log.Errorf("Flywheel: distill request failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Errorf("Flywheel: distill request returned %d: %s", resp.StatusCode, string(respBody))
		return
	}

	var distillResp distillResponse
	if err := json.NewDecoder(resp.Body).Decode(&distillResp); err != nil {
		log.Errorf("Flywheel: failed to decode distill response: %v", err)
		return
	}

	log.Infof("Flywheel: distillation for %s=%s returned %d candidate rules",
		category, category, len(distillResp.CandidateRules))

	// Store candidate rules in DB
	storeCandidateRules(ctx, distillResp.CandidateRules)
}

// storeCandidateRules persists new candidate rules to the intent_rule table
func storeCandidateRules(ctx context.Context, rules []candidateRule) {
	ruleFacade := database.NewIntentRuleFacade()

	for _, cr := range rules {
		// Check if a similar rule already exists
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

// TriggerConductorAudit sends a batch of rule-matched workloads
// to Conductor for LLM-powered audit verification.
func TriggerConductorAudit(
	ctx context.Context,
	conductorURL string,
	samples []auditSamplePayload,
) (*auditResponsePayload, error) {
	body, err := json.Marshal(map[string]interface{}{
		"samples": samples,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal audit request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/intent/audit", conductorURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create audit request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("audit request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("audit returned %d: %s", resp.StatusCode, string(respBody))
	}

	var auditResp auditResponsePayload
	if err := json.NewDecoder(resp.Body).Decode(&auditResp); err != nil {
		return nil, fmt.Errorf("failed to decode audit response: %w", err)
	}

	return &auditResp, nil
}

// auditSamplePayload matches the Conductor AuditSample schema
type auditSamplePayload struct {
	WorkloadUID      string                 `json:"workload_uid"`
	RuleID           int64                  `json:"rule_id"`
	RulePattern      string                 `json:"rule_pattern"`
	RuleDimension    string                 `json:"rule_dimension"`
	RuleDetectsField string                 `json:"rule_detects_field"`
	RuleDetectsValue string                 `json:"rule_detects_value"`
	CurrentCategory  string                 `json:"current_category"`
	CurrentIntentDetail map[string]interface{} `json:"current_intent_detail"`
	Evidence         map[string]interface{} `json:"evidence"`
}

// auditResponsePayload matches the Conductor AuditResponse schema
type auditResponsePayload struct {
	TotalAudited      int              `json:"total_audited"`
	ConsistentCount   int              `json:"consistent_count"`
	InconsistentCount int              `json:"inconsistent_count"`
	Verdicts          []auditVerdict   `json:"verdicts"`
}

type auditVerdict struct {
	WorkloadUID       string                 `json:"workload_uid"`
	RuleID            int64                  `json:"rule_id"`
	Consistent        bool                   `json:"consistent"`
	LLMCategory       string                 `json:"llm_category"`
	LLMIntentDetail   map[string]interface{} `json:"llm_intent_detail"`
	DiscrepancyFields []string               `json:"discrepancy_fields"`
	Explanation       string                 `json:"explanation"`
	Confidence        float64                `json:"confidence"`
}
