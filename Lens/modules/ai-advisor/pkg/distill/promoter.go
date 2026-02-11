// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package distill

import (
	"context"
	"encoding/json"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	// PromotionPrecisionThreshold is the minimum precision to promote a rule
	PromotionPrecisionThreshold = 0.90

	// PromotionRecallThreshold is the minimum recall to promote a rule
	PromotionRecallThreshold = 0.30

	// PromotionMinSamples is the minimum backtest sample count to promote
	PromotionMinSamples = 20

	// RetirementFPRateThreshold: retire if false positive rate exceeds this
	RetirementFPRateThreshold = 0.15

	// RetirementMinAudits: minimum audits before considering retirement
	RetirementMinAudits = 10
)

// Promoter handles rule promotion (validated -> promoted) and retirement
// (promoted -> retired) based on backtest metrics and audit results.
type Promoter struct {
	ruleFacade database.IntentRuleFacadeInterface
}

// NewPromoter creates a new Promoter
func NewPromoter() *Promoter {
	return &Promoter{
		ruleFacade: database.NewIntentRuleFacade(),
	}
}

// PromoteValidatedRules promotes all validated rules that meet criteria
func (p *Promoter) PromoteValidatedRules(ctx context.Context) (int, error) {
	rules, err := p.ruleFacade.ListByStatus(ctx, "validated")
	if err != nil {
		return 0, err
	}

	promoted := 0
	for _, rule := range rules {
		if p.shouldPromote(rule) {
			if err := p.promote(ctx, rule); err != nil {
				log.Errorf("Promoter: failed to promote rule %d: %v", rule.ID, err)
				continue
			}
			promoted++
		}
	}

	log.Infof("Promoter: promoted %d rules out of %d validated", promoted, len(rules))
	return promoted, nil
}

// RetireUnderperformingRules retires promoted rules with high false positive rates
func (p *Promoter) RetireUnderperformingRules(ctx context.Context) (int, error) {
	rules, err := p.ruleFacade.ListByStatus(ctx, "promoted")
	if err != nil {
		return 0, err
	}

	retired := 0
	for _, rule := range rules {
		if p.shouldRetire(rule) {
			if err := p.retire(ctx, rule); err != nil {
				log.Errorf("Promoter: failed to retire rule %d: %v", rule.ID, err)
				continue
			}
			retired++
		}
	}

	if retired > 0 {
		log.Warnf("Promoter: retired %d underperforming rules", retired)
	}
	return retired, nil
}

// RunPromotionCycle runs a full promotion + retirement cycle
func (p *Promoter) RunPromotionCycle(ctx context.Context) error {
	promoted, err := p.PromoteValidatedRules(ctx)
	if err != nil {
		return err
	}

	retired, err := p.RetireUnderperformingRules(ctx)
	if err != nil {
		return err
	}

	log.Infof("Promoter: cycle complete (promoted=%d, retired=%d)", promoted, retired)
	return nil
}

// ForcePromote manually promotes a rule (admin action)
func (p *Promoter) ForcePromote(ctx context.Context, ruleID int64) error {
	rule, err := p.ruleFacade.GetRule(ctx, ruleID)
	if err != nil {
		return err
	}
	if rule == nil {
		log.Warnf("Promoter: rule %d not found", ruleID)
		return nil
	}

	return p.promote(ctx, rule)
}

// ForceRetire manually retires a rule (admin action)
func (p *Promoter) ForceRetire(ctx context.Context, ruleID int64) error {
	rule, err := p.ruleFacade.GetRule(ctx, ruleID)
	if err != nil {
		return err
	}
	if rule == nil {
		log.Warnf("Promoter: rule %d not found", ruleID)
		return nil
	}

	return p.retire(ctx, rule)
}

// GetPromotedRules returns all currently promoted rules (for the confidence router)
func (p *Promoter) GetPromotedRules(ctx context.Context) ([]*model.IntentRule, error) {
	return p.ruleFacade.ListByStatus(ctx, "promoted")
}

// shouldPromote checks if a validated rule meets promotion criteria
func (p *Promoter) shouldPromote(rule *model.IntentRule) bool {
	// Parse backtest result
	bt := p.parseBacktestResult(rule)
	if bt == nil {
		return false
	}

	// Must have enough samples
	if bt.SampleCount < PromotionMinSamples {
		return false
	}

	// Must meet precision threshold
	if bt.Precision < PromotionPrecisionThreshold {
		return false
	}

	// Must have reasonable recall
	if bt.Recall < PromotionRecallThreshold {
		return false
	}

	// Must have been backtested recently (within 7 days)
	if rule.LastBacktestedAt == nil || time.Since(*rule.LastBacktestedAt) > 7*24*time.Hour {
		return false
	}

	return true
}

// shouldRetire checks if a promoted rule should be retired
func (p *Promoter) shouldRetire(rule *model.IntentRule) bool {
	totalAudits := int(rule.CorrectCount + rule.FalsePositiveCount)

	// Not enough audits to judge
	if totalAudits < RetirementMinAudits {
		return false
	}

	// Calculate false positive rate
	fpRate := float64(rule.FalsePositiveCount) / float64(totalAudits)

	return fpRate > RetirementFPRateThreshold
}

func (p *Promoter) promote(ctx context.Context, rule *model.IntentRule) error {
	log.Infof("Promoter: promoting rule %d (%s=%s, pattern=%q)",
		rule.ID, rule.DetectsField, rule.DetectsValue, rule.Pattern)

	return p.ruleFacade.UpdateStatus(ctx, rule.ID, "promoted")
}

func (p *Promoter) retire(ctx context.Context, rule *model.IntentRule) error {
	log.Warnf("Promoter: retiring rule %d (%s=%s) - FP=%d, Correct=%d",
		rule.ID, rule.DetectsField, rule.DetectsValue,
		rule.FalsePositiveCount, rule.CorrectCount)

	return p.ruleFacade.UpdateStatus(ctx, rule.ID, "retired")
}

func (p *Promoter) parseBacktestResult(rule *model.IntentRule) *BacktestResult {
	if rule.BacktestResult == nil {
		return nil
	}

	resultJSON, err := json.Marshal(rule.BacktestResult)
	if err != nil {
		return nil
	}

	var bt BacktestResult
	if err := json.Unmarshal(resultJSON, &bt); err != nil {
		return nil
	}

	return &bt
}
