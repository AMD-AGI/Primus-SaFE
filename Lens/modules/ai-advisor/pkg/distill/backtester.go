// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package distill

import (
	"context"
	"encoding/json"
	"regexp"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// BacktestResult contains metrics from backtesting a candidate rule
type BacktestResult struct {
	Precision   float64 `json:"precision"`
	Recall      float64 `json:"recall"`
	F1          float64 `json:"f1"`
	TP          int     `json:"tp"`           // True positives
	FP          int     `json:"fp"`           // False positives
	FN          int     `json:"fn"`           // False negatives
	TN          int     `json:"tn"`           // True negatives
	SampleCount int     `json:"sample_count"` // Total samples tested
}

// Backtester validates candidate rules against historical workload data.
// It computes precision, recall, and F1 by comparing the rule's regex match
// against the confirmed intent values in workload_detection.
type Backtester struct {
	detectionFacade database.WorkloadDetectionFacadeInterface
	evidenceFacade  database.WorkloadDetectionEvidenceFacadeInterface
	ruleFacade      database.IntentRuleFacadeInterface
}

// NewBacktester creates a new Backtester
func NewBacktester() *Backtester {
	return &Backtester{
		detectionFacade: database.NewWorkloadDetectionFacade(),
		evidenceFacade:  database.NewWorkloadDetectionEvidenceFacade(),
		ruleFacade:      database.NewIntentRuleFacade(),
	}
}

// BacktestRule runs backtesting for a single candidate rule.
// It tests the rule's regex pattern against all confirmed workloads
// in the historical data.
func (b *Backtester) BacktestRule(ctx context.Context, rule *model.IntentRule) (*BacktestResult, error) {
	// Compile the regex
	re, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return nil, err
	}

	// Get all confirmed workloads (those with confirmed intent state)
	detections, _, err := b.detectionFacade.ListByIntentState(ctx,
		"confirmed", // only confirmed intent
		500,         // limit
		0,           // offset
	)
	if err != nil {
		return nil, err
	}

	result := &BacktestResult{
		SampleCount: len(detections),
	}

	for _, det := range detections {
		// Get the evidence value to match against (based on dimension)
		matchValue := b.getMatchValue(ctx, det, rule.Dimension)

		// Get the ground truth value for the field this rule detects
		groundTruth := b.getGroundTruth(det, rule.DetectsField)

		matched := matchValue != "" && re.MatchString(matchValue)
		isPositive := groundTruth == rule.DetectsValue

		switch {
		case matched && isPositive:
			result.TP++
		case matched && !isPositive:
			result.FP++
		case !matched && isPositive:
			result.FN++
		case !matched && !isPositive:
			result.TN++
		}
	}

	// Calculate metrics
	if result.TP+result.FP > 0 {
		result.Precision = float64(result.TP) / float64(result.TP+result.FP)
	}
	if result.TP+result.FN > 0 {
		result.Recall = float64(result.TP) / float64(result.TP+result.FN)
	}
	if result.Precision+result.Recall > 0 {
		result.F1 = 2 * result.Precision * result.Recall / (result.Precision + result.Recall)
	}

	return result, nil
}

// BacktestAndUpdate runs backtest and updates the rule record in DB
func (b *Backtester) BacktestAndUpdate(ctx context.Context, ruleID int64) error {
	rule, err := b.ruleFacade.GetRule(ctx, ruleID)
	if err != nil {
		return err
	}
	if rule == nil {
		log.Warnf("Backtester: rule %d not found", ruleID)
		return nil
	}

	result, err := b.BacktestRule(ctx, rule)
	if err != nil {
		log.Errorf("Backtester: failed to backtest rule %d: %v", ruleID, err)
		return err
	}

	// Update rule with backtest results
	resultMap := map[string]interface{}{
		"precision":    result.Precision,
		"recall":       result.Recall,
		"f1":           result.F1,
		"tp":           result.TP,
		"fp":           result.FP,
		"fn":           result.FN,
		"tn":           result.TN,
		"sample_count": result.SampleCount,
	}
	if err := b.ruleFacade.UpdateBacktestResult(ctx, ruleID, resultMap); err != nil {
		return err
	}

	// Transition status based on results
	if rule.Status == "proposed" && result.SampleCount >= 10 {
		if result.Precision >= 0.90 && result.Recall >= 0.30 {
			log.Infof("Backtester: rule %d validated (P=%.2f, R=%.2f, F1=%.2f)",
				ruleID, result.Precision, result.Recall, result.F1)
			return b.ruleFacade.UpdateStatus(ctx, ruleID, "validated")
		} else if result.Precision < 0.50 {
			log.Infof("Backtester: rule %d rejected (P=%.2f too low)", ruleID, result.Precision)
			return b.ruleFacade.UpdateStatus(ctx, ruleID, "rejected")
		} else {
			return b.ruleFacade.UpdateStatus(ctx, ruleID, "testing")
		}
	}

	return nil
}

// BacktestAll runs backtesting for all rules in "proposed" or "testing" status
func (b *Backtester) BacktestAll(ctx context.Context) (int, error) {
	rules, err := b.ruleFacade.ListByStatus(ctx, "proposed")
	if err != nil {
		return 0, err
	}

	testingRules, err := b.ruleFacade.ListByStatus(ctx, "testing")
	if err != nil {
		return 0, err
	}
	rules = append(rules, testingRules...)

	count := 0
	for _, rule := range rules {
		if err := b.BacktestAndUpdate(ctx, rule.ID); err != nil {
			log.Errorf("Backtester: error backtesting rule %d: %v", rule.ID, err)
			continue
		}
		count++
	}

	log.Infof("Backtester: completed backtesting %d rules", count)
	return count, nil
}

// getMatchValue extracts the value to match against based on the rule dimension.
// It queries detection evidence for the workload and concatenates relevant data.
func (b *Backtester) getMatchValue(ctx context.Context, det *model.WorkloadDetection, dimension string) string {
	evidences, err := b.evidenceFacade.ListEvidenceByWorkload(ctx, det.WorkloadUID)
	if err != nil || len(evidences) == 0 {
		return ""
	}

	// Concatenate all evidence data for the matching dimension
	var parts []string
	for _, ev := range evidences {
		if ev.RawData == nil {
			continue
		}
		rawJSON, _ := json.Marshal(ev.RawData)
		var raw map[string]interface{}
		if json.Unmarshal(rawJSON, &raw) != nil {
			continue
		}

		switch dimension {
		case "cmdline":
			if cmd, ok := raw["cmdline"].(string); ok {
				parts = append(parts, cmd)
			}
			if cmds, ok := raw["cmdlines"].([]interface{}); ok {
				for _, c := range cmds {
					if s, ok := c.(string); ok {
						parts = append(parts, s)
					}
				}
			}
		case "image":
			if img, ok := raw["image"].(string); ok {
				parts = append(parts, img)
			}
		case "env_key":
			if envMap, ok := raw["env"].(map[string]interface{}); ok {
				for k := range envMap {
					parts = append(parts, k)
				}
			}
		case "env_value":
			if envMap, ok := raw["env"].(map[string]interface{}); ok {
				for _, v := range envMap {
					if s, ok := v.(string); ok {
						parts = append(parts, s)
					}
				}
			}
		case "pip":
			if pipFreeze, ok := raw["pip_freeze"].(string); ok {
				parts = append(parts, pipFreeze)
			}
		case "config":
			if configs, ok := raw["configs"].(map[string]interface{}); ok {
				for _, v := range configs {
					if s, ok := v.(string); ok {
						parts = append(parts, s)
					}
				}
			}
		case "code":
			if entry, ok := raw["entry_script"].(map[string]interface{}); ok {
				if content, ok := entry["content"].(string); ok {
					parts = append(parts, content)
				}
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}
	// Join all parts with newline for regex matching
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "\n"
		}
		result += p
	}
	return result
}

// getGroundTruth extracts the confirmed ground truth for a field
func (b *Backtester) getGroundTruth(det *model.WorkloadDetection, field string) string {
	switch field {
	case "category":
		if det.Category != nil {
			return *det.Category
		}
	case "model_family":
		if det.ModelFamily != nil {
			return *det.ModelFamily
		}
	case "model_scale":
		if det.ModelScale != nil {
			return *det.ModelScale
		}
	case "training_method":
		if det.IntentDetail != nil {
			detailJSON, _ := json.Marshal(det.IntentDetail)
			var detail map[string]interface{}
			if json.Unmarshal(detailJSON, &detail) == nil {
				if method, ok := detail["method"].(string); ok {
					return method
				}
			}
		}
	}
	return ""
}
