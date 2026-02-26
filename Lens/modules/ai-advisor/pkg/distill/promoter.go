// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package distill

import (
	"context"
	"encoding/json"
	"regexp"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	// PromotionPrecisionThreshold is the minimum precision to promote a rule
	PromotionPrecisionThreshold = 0.90

	// PromotionRecallThreshold is the minimum recall to promote a rule
	PromotionRecallThreshold = 0.10

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

// PromoteValidatedRules promotes all validated rules that meet criteria,
// skipping rules that are semantically redundant with already-promoted ones.
func (p *Promoter) PromoteValidatedRules(ctx context.Context) (int, error) {
	rules, err := p.ruleFacade.ListByStatus(ctx, "validated")
	if err != nil {
		return 0, err
	}

	existingPromoted, err := p.ruleFacade.ListByStatus(ctx, "promoted")
	if err != nil {
		existingPromoted = nil
	}

	promoted := 0
	skippedDup := 0
	for _, rule := range rules {
		if !p.shouldPromote(rule) {
			continue
		}

		if p.isDuplicateOfExisting(rule, existingPromoted) {
			log.Infof("Promoter: skipping duplicate rule %d (%s/%s pattern=%q), already covered by promoted rules",
				rule.ID, rule.DetectsValue, rule.Dimension, rule.Pattern)
			_ = p.ruleFacade.UpdateStatus(ctx, rule.ID, "rejected")
			skippedDup++
			continue
		}

		if err := p.promote(ctx, rule); err != nil {
			log.Errorf("Promoter: failed to promote rule %d: %v", rule.ID, err)
			continue
		}
		existingPromoted = append(existingPromoted, rule)
		promoted++
	}

	log.Infof("Promoter: promoted %d rules, skipped %d duplicates, out of %d validated",
		promoted, skippedDup, len(rules))
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

// isDuplicateOfExisting checks if a candidate rule is semantically redundant
// with an already-promoted rule (same detects_value + dimension and overlapping
// regex pattern that would match the same inputs).
func (p *Promoter) isDuplicateOfExisting(candidate *model.IntentRule, promoted []*model.IntentRule) bool {
	candidateRe, err := regexp.Compile(candidate.Pattern)
	if err != nil {
		return false
	}

	for _, existing := range promoted {
		if existing.DetectsValue != candidate.DetectsValue || existing.Dimension != candidate.Dimension {
			continue
		}

		existingRe, err := regexp.Compile(existing.Pattern)
		if err != nil {
			continue
		}

		// Check mutual subsumption using the candidate's own derived_from workload UIDs
		// as test corpus: if the existing rule matches everything the candidate matches,
		// the candidate is redundant.
		testStrings := extractPatternAlternatives(candidate.Pattern)
		testStrings = append(testStrings, extractPatternAlternatives(existing.Pattern)...)

		if len(testStrings) == 0 {
			// Fallback: exact pattern match
			if candidate.Pattern == existing.Pattern {
				return true
			}
			continue
		}

		candidateMatches := 0
		existingAlsoMatches := 0
		for _, ts := range testStrings {
			if candidateRe.MatchString(ts) {
				candidateMatches++
				if existingRe.MatchString(ts) {
					existingAlsoMatches++
				}
			}
		}

		// If existing rule covers >=80% of what candidate matches, it's redundant
		if candidateMatches > 0 && float64(existingAlsoMatches)/float64(candidateMatches) >= 0.8 {
			return true
		}
	}
	return false
}

// extractPatternAlternatives extracts literal alternatives from a regex pattern
// e.g. `\b(foo|bar|baz)\b` -> ["foo", "bar", "baz"]
func extractPatternAlternatives(pattern string) []string {
	altRe := regexp.MustCompile(`\(([^()]+)\)`)
	matches := altRe.FindAllStringSubmatch(pattern, -1)
	var results []string
	for _, m := range matches {
		if len(m) > 1 {
			parts := regexp.MustCompile(`\|`).Split(m[1], -1)
			for _, part := range parts {
				cleaned := regexp.MustCompile(`\\[bBwWdDsS]|[?+*^$\[\]]|\\.`).ReplaceAllString(part, "")
				cleaned = regexp.MustCompile(`\[[-\]\\]+\]`).ReplaceAllString(cleaned, "-")
				if len(cleaned) >= 3 {
					results = append(results, cleaned)
				}
			}
		}
	}
	return results
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
