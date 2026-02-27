// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package ahocorasick implements the Aho-Corasick multi-pattern string matching
// algorithm. It constructs a finite state automaton from a set of keyword patterns
// and scans input text in O(n + m) time where n is the text length and m is the
// total number of pattern occurrences.
//
// This is used by the telemetry-processor for real-time, framework-agnostic log
// pattern matching in the workload intent analysis pipeline.
package ahocorasick

// node represents a state in the Aho-Corasick automaton
type node struct {
	children map[byte]*node
	fail     *node
	output   []int // indices into the pattern list
	depth    int
}

// Pattern represents a search pattern with associated metadata
type Pattern struct {
	Keyword    string  // The literal string to match
	ID         int64   // External identifier (e.g., intent_rule.id)
	Field      string  // Which field this pattern detects (e.g., "category", "model_family")
	Value      string  // The detected value when this pattern matches
	Confidence float64 // Confidence score when this pattern matches
}

// Match represents a single pattern match in the input text
type Match struct {
	Pattern  *Pattern // The matched pattern
	Position int      // Byte offset in the input where the match ends
}

// Automaton is an Aho-Corasick finite state automaton for multi-pattern matching
type Automaton struct {
	root     *node
	patterns []*Pattern
	built    bool
}

// New creates a new empty Automaton. Add patterns with AddPattern, then call Build.
func New() *Automaton {
	return &Automaton{
		root: &node{
			children: make(map[byte]*node),
		},
	}
}

// AddPattern adds a keyword pattern to the automaton.
// Must be called before Build.
func (a *Automaton) AddPattern(p *Pattern) {
	if a.built {
		panic("cannot add patterns after Build")
	}
	idx := len(a.patterns)
	a.patterns = append(a.patterns, p)

	current := a.root
	for i := 0; i < len(p.Keyword); i++ {
		ch := p.Keyword[i]
		child, ok := current.children[ch]
		if !ok {
			child = &node{
				children: make(map[byte]*node),
				depth:    current.depth + 1,
			}
			current.children[ch] = child
		}
		current = child
	}
	current.output = append(current.output, idx)
}

// Build constructs the failure links and finalizes the automaton.
// Must be called after all patterns are added and before Search.
func (a *Automaton) Build() {
	if a.built {
		return
	}
	a.built = true

	// BFS to build failure links
	queue := make([]*node, 0)

	// Initialize failure links for depth-1 nodes
	for _, child := range a.root.children {
		child.fail = a.root
		queue = append(queue, child)
	}

	// BFS
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for ch, child := range current.children {
			queue = append(queue, child)

			// Follow failure links to find the longest proper suffix
			fail := current.fail
			for fail != nil {
				if next, ok := fail.children[ch]; ok {
					child.fail = next
					break
				}
				fail = fail.fail
			}
			if child.fail == nil {
				child.fail = a.root
			}

			// Merge output from failure link (dictionary suffix links)
			if child.fail != a.root && len(child.fail.output) > 0 {
				child.output = append(child.output, child.fail.output...)
			}
		}
	}
}

// Search scans the input text and returns all pattern matches.
// The automaton must be built (via Build) before calling Search.
func (a *Automaton) Search(text string) []Match {
	if !a.built {
		panic("automaton not built; call Build() first")
	}

	var matches []Match
	current := a.root

	for i := 0; i < len(text); i++ {
		ch := text[i]

		// Follow failure links until we find a matching transition or reach root
		for current != a.root {
			if _, ok := current.children[ch]; ok {
				break
			}
			current = current.fail
		}

		if next, ok := current.children[ch]; ok {
			current = next
		}

		// Collect all outputs at this state
		if len(current.output) > 0 {
			for _, idx := range current.output {
				matches = append(matches, Match{
					Pattern:  a.patterns[idx],
					Position: i,
				})
			}
		}
	}

	return matches
}

// SearchBytes scans byte input and returns all pattern matches.
func (a *Automaton) SearchBytes(data []byte) []Match {
	return a.Search(string(data))
}

// PatternCount returns the number of patterns in the automaton
func (a *Automaton) PatternCount() int {
	return len(a.patterns)
}

// HasPatterns returns true if the automaton has any patterns
func (a *Automaton) HasPatterns() bool {
	return len(a.patterns) > 0
}
