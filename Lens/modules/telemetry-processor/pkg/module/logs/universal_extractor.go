// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package logs

import (
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/ahocorasick"
	"github.com/sirupsen/logrus"
)

// UniversalExtractor uses an Aho-Corasick automaton to perform framework-agnostic
// log pattern matching in real-time. It replaces per-framework regex matchers with
// a single automaton that processes all patterns simultaneously in O(n) time.
//
// The extractor is designed to be used in the telemetry-processor's log pipeline:
//   - Patterns are loaded from the intent_rule table (promoted rules)
//   - The automaton is rebuilt periodically to pick up new/updated rules
//   - Each log line is scanned once against all patterns
type UniversalExtractor struct {
	mu       sync.RWMutex
	ac       *ahocorasick.Automaton
	builtAt  time.Time
	patterns []*ahocorasick.Pattern

	// Statistics
	linesProcessed int64
	totalMatches   int64
}

// NewUniversalExtractor creates a new extractor with an empty automaton.
// Call LoadPatterns to populate it.
func NewUniversalExtractor() *UniversalExtractor {
	return &UniversalExtractor{
		ac: ahocorasick.New(),
	}
}

// LoadPatterns loads patterns and rebuilds the automaton.
// This can be called periodically to refresh rules from the database.
func (e *UniversalExtractor) LoadPatterns(patterns []*ahocorasick.Pattern) {
	e.mu.Lock()
	defer e.mu.Unlock()

	builder := ahocorasick.NewBuilder().CaseInsensitive()
	builder.AddPatterns(patterns)
	e.ac = builder.Build()
	e.patterns = patterns
	e.builtAt = time.Now()

	logrus.Infof("UniversalExtractor: loaded %d patterns", len(patterns))
}

// ExtractionResult holds the result of processing a single log line
type ExtractionResult struct {
	Matches    []ExtractedSignal
	LineLength int
}

// ExtractedSignal represents a single signal extracted from a log line
type ExtractedSignal struct {
	RuleID     int64   // intent_rule.id that matched
	Field      string  // e.g., "category", "model_family"
	Value      string  // e.g., "pre_training", "llama"
	Confidence float64 // Match confidence
	Position   int     // Position in the log line
}

// ProcessLine scans a single log line against all patterns
func (e *UniversalExtractor) ProcessLine(line string) *ExtractionResult {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := &ExtractionResult{
		LineLength: len(line),
	}

	if e.ac == nil || !e.ac.HasPatterns() {
		return result
	}

	matches := e.ac.Search(line)
	e.linesProcessed++

	for _, m := range matches {
		result.Matches = append(result.Matches, ExtractedSignal{
			RuleID:     m.Pattern.ID,
			Field:      m.Pattern.Field,
			Value:      m.Pattern.Value,
			Confidence: m.Pattern.Confidence,
			Position:   m.Position,
		})
	}

	e.totalMatches += int64(len(result.Matches))

	return result
}

// ProcessBatch scans multiple log lines and returns aggregated results
func (e *UniversalExtractor) ProcessBatch(lines []string) map[string]*AggregatedSignal {
	signals := make(map[string]*AggregatedSignal)

	for _, line := range lines {
		result := e.ProcessLine(line)
		for _, sig := range result.Matches {
			key := sig.Field + ":" + sig.Value
			agg, ok := signals[key]
			if !ok {
				agg = &AggregatedSignal{
					Field:    sig.Field,
					Value:    sig.Value,
					RuleIDs:  make(map[int64]bool),
				}
				signals[key] = agg
			}
			agg.Count++
			agg.RuleIDs[sig.RuleID] = true
			if sig.Confidence > agg.MaxConfidence {
				agg.MaxConfidence = sig.Confidence
			}
		}
	}

	return signals
}

// AggregatedSignal represents accumulated matches for a specific field:value pair
type AggregatedSignal struct {
	Field         string
	Value         string
	Count         int
	MaxConfidence float64
	RuleIDs       map[int64]bool
}

// Stats returns extractor statistics
type ExtractorStats struct {
	PatternCount   int
	LinesProcessed int64
	TotalMatches   int64
	BuiltAt        time.Time
}

// GetStats returns current statistics
func (e *UniversalExtractor) GetStats() ExtractorStats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return ExtractorStats{
		PatternCount:   len(e.patterns),
		LinesProcessed: e.linesProcessed,
		TotalMatches:   e.totalMatches,
		BuiltAt:        e.builtAt,
	}
}

// HasPatterns returns true if the extractor has patterns loaded
func (e *UniversalExtractor) HasPatterns() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ac != nil && e.ac.HasPatterns()
}
