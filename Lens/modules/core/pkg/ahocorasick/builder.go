// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package ahocorasick

import (
	"strings"
)

// CaseInsensitiveAutomaton wraps Automaton with case-insensitive matching.
// It lowercases all patterns during construction and lowercases input during search.
type CaseInsensitiveAutomaton struct {
	inner *Automaton
}

// NewCaseInsensitive creates a case-insensitive automaton
func NewCaseInsensitive() *CaseInsensitiveAutomaton {
	return &CaseInsensitiveAutomaton{
		inner: New(),
	}
}

// AddPattern adds a pattern (keyword is lowercased internally)
func (a *CaseInsensitiveAutomaton) AddPattern(p *Pattern) {
	lowered := &Pattern{
		Keyword:    strings.ToLower(p.Keyword),
		ID:         p.ID,
		Field:      p.Field,
		Value:      p.Value,
		Confidence: p.Confidence,
	}
	a.inner.AddPattern(lowered)
}

// Build finalizes the automaton
func (a *CaseInsensitiveAutomaton) Build() {
	a.inner.Build()
}

// Search performs case-insensitive search
func (a *CaseInsensitiveAutomaton) Search(text string) []Match {
	return a.inner.Search(strings.ToLower(text))
}

// PatternCount returns the number of patterns
func (a *CaseInsensitiveAutomaton) PatternCount() int {
	return a.inner.PatternCount()
}

// HasPatterns returns true if patterns exist
func (a *CaseInsensitiveAutomaton) HasPatterns() bool {
	return a.inner.HasPatterns()
}

// Builder provides a fluent API for constructing automatons from various sources
type Builder struct {
	patterns      []*Pattern
	caseInsensitive bool
}

// NewBuilder creates a new Builder
func NewBuilder() *Builder {
	return &Builder{}
}

// CaseInsensitive enables case-insensitive matching
func (b *Builder) CaseInsensitive() *Builder {
	b.caseInsensitive = true
	return b
}

// AddKeyword adds a simple keyword pattern
func (b *Builder) AddKeyword(keyword string, id int64, field, value string, confidence float64) *Builder {
	b.patterns = append(b.patterns, &Pattern{
		Keyword:    keyword,
		ID:         id,
		Field:      field,
		Value:      value,
		Confidence: confidence,
	})
	return b
}

// AddPatterns adds multiple patterns at once
func (b *Builder) AddPatterns(patterns []*Pattern) *Builder {
	b.patterns = append(b.patterns, patterns...)
	return b
}

// Build constructs and returns the automaton
func (b *Builder) Build() *Automaton {
	if b.caseInsensitive {
		ci := NewCaseInsensitive()
		for _, p := range b.patterns {
			ci.AddPattern(p)
		}
		ci.Build()
		return ci.inner
	}

	a := New()
	for _, p := range b.patterns {
		a.AddPattern(p)
	}
	a.Build()
	return a
}
