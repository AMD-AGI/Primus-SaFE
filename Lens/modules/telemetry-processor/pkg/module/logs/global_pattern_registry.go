// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package logs

import (
	"context"
	"regexp"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// GlobalPatternRegistry holds all compiled log parsing patterns.
// Patterns are loaded from the training_log_pattern table and matched
// against ALL incoming logs regardless of framework.
type GlobalPatternRegistry struct {
	mu sync.RWMutex

	performancePatterns   []*CompiledGlobalPattern
	blacklistPatterns     []*CompiledGlobalPattern
	trainingEventPatterns map[string][]*CompiledGlobalPattern // keyed by event_subtype
	checkpointPatterns    map[string][]*CompiledGlobalPattern // keyed by event_subtype

	lastLoadedAt time.Time
	patternCount int
	facade       database.TrainingLogPatternFacadeInterface
}

// CompiledGlobalPattern wraps a compiled regex with metadata from training_log_pattern
type CompiledGlobalPattern struct {
	ID         int64
	Name       string
	Pattern    *regexp.Regexp
	PatternStr string
	Priority   int
	Confidence float64
	Framework  string // informational only
}

// GlobalMatchResult represents the result of matching a log line against global patterns
type GlobalMatchResult struct {
	Matched   bool
	PatternID int64
	Pattern   string
	Groups    map[string]string
	Framework string // informational
}

// NewGlobalPatternRegistry creates a new registry
func NewGlobalPatternRegistry() *GlobalPatternRegistry {
	return &GlobalPatternRegistry{
		trainingEventPatterns: make(map[string][]*CompiledGlobalPattern),
		checkpointPatterns:    make(map[string][]*CompiledGlobalPattern),
		facade:                database.NewTrainingLogPatternFacade(),
	}
}

// Load loads all enabled patterns from the database
func (r *GlobalPatternRegistry) Load(ctx context.Context) error {
	all, err := r.facade.ListAllEnabled(ctx)
	if err != nil {
		return err
	}

	var (
		perf      []*CompiledGlobalPattern
		blacklist []*CompiledGlobalPattern
		trainEvt  = make(map[string][]*CompiledGlobalPattern)
		ckptEvt   = make(map[string][]*CompiledGlobalPattern)
		compiled  int
		failed    int
	)

	for _, p := range all {
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			logrus.Warnf("GlobalPatternRegistry: failed to compile pattern id=%d name=%v: %v", p.ID, p.Name, err)
			failed++
			continue
		}

		cp := &CompiledGlobalPattern{
			ID:         p.ID,
			Name:       derefStr(p.Name),
			Pattern:    re,
			PatternStr: p.Pattern,
			Priority:   p.Priority,
			Confidence: p.Confidence,
			Framework:  derefStr(p.Framework),
		}

		switch p.PatternType {
		case "performance":
			perf = append(perf, cp)
		case "blacklist":
			blacklist = append(blacklist, cp)
		case "training_event":
			subtype := derefStr(p.EventSubtype)
			if subtype != "" {
				trainEvt[subtype] = append(trainEvt[subtype], cp)
			}
		case "checkpoint_event":
			subtype := derefStr(p.EventSubtype)
			if subtype != "" {
				ckptEvt[subtype] = append(ckptEvt[subtype], cp)
			}
		}
		compiled++
	}

	r.mu.Lock()
	r.performancePatterns = perf
	r.blacklistPatterns = blacklist
	r.trainingEventPatterns = trainEvt
	r.checkpointPatterns = ckptEvt
	r.lastLoadedAt = time.Now()
	r.patternCount = compiled
	r.mu.Unlock()

	logrus.Infof("GlobalPatternRegistry: loaded %d patterns (%d failed) - %d performance, %d blacklist, %d training_event subtypes, %d checkpoint_event subtypes",
		compiled, failed, len(perf), len(blacklist), len(trainEvt), len(ckptEvt))

	return nil
}

// StartAutoReload starts a background goroutine that reloads patterns when changes are detected
func (r *GlobalPatternRegistry) StartAutoReload(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mu.RLock()
			since := r.lastLoadedAt
			r.mu.RUnlock()

			changed, err := r.facade.HasChangesSince(ctx, since)
			if err != nil {
				logrus.Warnf("GlobalPatternRegistry: failed to check for changes: %v", err)
				continue
			}
			if changed {
				logrus.Info("GlobalPatternRegistry: changes detected, reloading patterns")
				if err := r.Load(ctx); err != nil {
					logrus.Warnf("GlobalPatternRegistry: reload failed: %v", err)
				}
			}
		}
	}
}

// MatchPerformance tries all performance patterns against the message.
// Returns the first match (patterns are ordered by priority DESC).
func (r *GlobalPatternRegistry) MatchPerformance(msg string) *GlobalMatchResult {
	r.mu.RLock()
	patterns := r.performancePatterns
	r.mu.RUnlock()

	for _, cp := range patterns {
		match := cp.Pattern.FindStringSubmatch(msg)
		if match != nil {
			groups := extractNamedGroups(cp.Pattern, match)
			// Async hit count update (fire-and-forget)
			go r.updateHitCount(cp.ID)
			return &GlobalMatchResult{
				Matched:   true,
				PatternID: cp.ID,
				Pattern:   cp.PatternStr,
				Groups:    groups,
				Framework: cp.Framework,
			}
		}
	}
	return &GlobalMatchResult{Matched: false}
}

// IsBlacklisted checks if the message matches any blacklist pattern
func (r *GlobalPatternRegistry) IsBlacklisted(msg string) bool {
	r.mu.RLock()
	patterns := r.blacklistPatterns
	r.mu.RUnlock()

	for _, cp := range patterns {
		if cp.Pattern.MatchString(msg) {
			return true
		}
	}
	return false
}

// MatchTrainingEvent tries all training event patterns.
// Returns the event subtype and match result.
func (r *GlobalPatternRegistry) MatchTrainingEvent(msg string) (string, *GlobalMatchResult) {
	r.mu.RLock()
	events := r.trainingEventPatterns
	r.mu.RUnlock()

	for subtype, patterns := range events {
		for _, cp := range patterns {
			match := cp.Pattern.FindStringSubmatch(msg)
			if match != nil {
				groups := extractNamedGroups(cp.Pattern, match)
				go r.updateHitCount(cp.ID)
				return subtype, &GlobalMatchResult{
					Matched:   true,
					PatternID: cp.ID,
					Pattern:   cp.PatternStr,
					Groups:    groups,
					Framework: cp.Framework,
				}
			}
		}
	}
	return "", &GlobalMatchResult{Matched: false}
}

// MatchCheckpointEvent tries all checkpoint event patterns.
// Returns the event subtype and match result.
func (r *GlobalPatternRegistry) MatchCheckpointEvent(msg string) (string, *GlobalMatchResult) {
	r.mu.RLock()
	events := r.checkpointPatterns
	r.mu.RUnlock()

	for subtype, patterns := range events {
		for _, cp := range patterns {
			match := cp.Pattern.FindStringSubmatch(msg)
			if match != nil {
				groups := extractNamedGroups(cp.Pattern, match)
				go r.updateHitCount(cp.ID)
				return subtype, &GlobalMatchResult{
					Matched:   true,
					PatternID: cp.ID,
					Pattern:   cp.PatternStr,
					Groups:    groups,
					Framework: cp.Framework,
				}
			}
		}
	}
	return "", &GlobalMatchResult{Matched: false}
}

// PatternCount returns the number of loaded patterns
func (r *GlobalPatternRegistry) PatternCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.patternCount
}

// GetDebugInfo returns pattern counts for debugging
func (r *GlobalPatternRegistry) GetDebugInfo() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	eventSubtypes := make([]string, 0)
	for k := range r.trainingEventPatterns {
		eventSubtypes = append(eventSubtypes, k)
	}
	ckptSubtypes := make([]string, 0)
	for k := range r.checkpointPatterns {
		ckptSubtypes = append(ckptSubtypes, k)
	}

	return map[string]interface{}{
		"total_patterns":          r.patternCount,
		"performance_patterns":    len(r.performancePatterns),
		"blacklist_patterns":      len(r.blacklistPatterns),
		"training_event_subtypes": eventSubtypes,
		"checkpoint_subtypes":     ckptSubtypes,
		"last_loaded_at":          r.lastLoadedAt,
	}
}

func (r *GlobalPatternRegistry) updateHitCount(id int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = r.facade.UpdateHitCount(ctx, id)
}

// extractNamedGroups extracts named capture groups from a regex match
func extractNamedGroups(pattern *regexp.Regexp, match []string) map[string]string {
	groups := make(map[string]string)
	for i, name := range pattern.SubexpNames() {
		if i > 0 && i < len(match) && name != "" && match[i] != "" {
			groups[name] = match[i]
		}
	}
	return groups
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Ensure the table is auto-migrated by referencing the model
var _ = model.TrainingLogPattern{}
