// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package logs

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// TestJSONEscaping tests the effect of JSON escaping on regular expressions
func TestJSONEscaping(t *testing.T) {
	log := `[20251202 09:12:08][rank-7/8][INFO ] [--------trainer.py:2560] : iteration 126/ 5000 | consumed samples: 16128 | elapsed time per iteration (ms): 13372.8/13364.7 | hip mem usage/free/total/usage_ratio: 153.81GB/102.17GB/255.98GB/60.09% | throughput per GPU (TFLOP/s/GPU): 567.6/568.0 | tokens per GPU (tokens/s/GPU): 9801.4/9807.3 | learning rate: 9.984820E-06 | global batch size: 128 | lm loss: 6.548988E-03 | loss scale: 1.0 | grad norm: 0.061 | number of skipped iterations: 0 | number of nan iterations: 0 |`

	// Test different levels of escaping
	tests := []struct {
		name        string
		jsonPattern string
		desc        string
	}{
		{
			name:        "Direct string (raw string literal)",
			jsonPattern: `.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)`,
			desc:        "Raw string literal with single backslash - this is the correct regex pattern",
		},
		{
			name:        "Regular string (double-quoted)",
			jsonPattern: ".*iteration\\s+(?P<CurrentIteration>\\d+)\\s*/\\s*(?P<TargetIteration>\\d+)",
			desc:        "Double-quoted string requires escaping backslashes",
		},
		{
			name:        "Simulating JSON-parsed string",
			// In raw string, we put what the result would be AFTER JSON parsing
			// JSON "\\s" becomes single \s after parsing
			jsonPattern: `.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)`,
			desc:        "After JSON unmarshaling, double backslash becomes single backslash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Description: %s", tt.desc)
			t.Logf("Pattern string: %s", tt.jsonPattern)
			t.Logf("Pattern length: %d", len(tt.jsonPattern))

			// Show escape characters
			showEscapes := strings.ReplaceAll(tt.jsonPattern, `\`, `[BACKSLASH]`)
			t.Logf("Pattern with escapes visible: %s", showEscapes[:min(200, len(showEscapes))])

			// Try to compile
			re, err := regexp.Compile(tt.jsonPattern)
			if err != nil {
				t.Errorf("❌ Failed to compile: %v", err)
				return
			}
			t.Logf("✓ Compiled successfully")

			// Try to match
			matches := re.FindStringSubmatch(log)
			if matches == nil {
				t.Errorf("❌ Pattern did not match")
				return
			}
			t.Logf("✓ Pattern matched, got %d groups", len(matches))

			// Extract named groups
			groups := make(map[string]string)
			names := re.SubexpNames()
			for i, name := range names {
				if i > 0 && i < len(matches) && name != "" {
					groups[name] = matches[i]
				}
			}

			t.Logf("Extracted %d named groups", len(groups))
			for name, value := range groups {
				t.Logf("  %s = %q", name, value)
			}

			// Validate values
			if groups["CurrentIteration"] != "126" {
				t.Errorf("❌ CurrentIteration: got %q, want \"126\"", groups["CurrentIteration"])
			} else {
				t.Logf("✓ CurrentIteration = 126")
			}
			if groups["TargetIteration"] != "5000" {
				t.Errorf("❌ TargetIteration: got %q, want \"5000\"", groups["TargetIteration"])
			} else {
				t.Logf("✓ TargetIteration = 5000")
			}
		})
	}

	// Now test complete JSON parsing flow
	t.Run("Complete JSON parsing flow", func(t *testing.T) {
		// This is the JSON as stored in database or config file
		// In JSON, backslashes must be escaped, so \s becomes \\s
		jsonStr := `{
			"pattern": ".*iteration\\s+(?P<CurrentIteration>\\d+)\\s*/\\s*(?P<TargetIteration>\\d+)"
		}`

		var config struct {
			Pattern string `json:"pattern"`
		}

		if err := json.Unmarshal([]byte(jsonStr), &config); err != nil {
			t.Fatalf("Failed to unmarshal JSON: %v", err)
		}

		t.Logf("After JSON parsing: %s", config.Pattern)
		showEscapes := strings.ReplaceAll(config.Pattern, `\`, `[BACKSLASH]`)
		t.Logf("With escapes visible: %s", showEscapes)

		// Compile and test
		re, err := regexp.Compile(config.Pattern)
		if err != nil {
			t.Fatalf("Failed to compile: %v", err)
		}

		matches := re.FindStringSubmatch(log)
		if matches == nil {
			t.Fatalf("Pattern did not match")
		}

		groups := make(map[string]string)
		names := re.SubexpNames()
		for i, name := range names {
			if i > 0 && i < len(matches) && name != "" {
				groups[name] = matches[i]
			}
		}

		t.Logf("Groups: %+v", groups)

		if groups["CurrentIteration"] != "126" {
			t.Errorf("❌ CurrentIteration: got %q, want \"126\"", groups["CurrentIteration"])
		} else {
			t.Logf("✓ CurrentIteration = 126")
		}
		
		if groups["TargetIteration"] != "5000" {
			t.Errorf("❌ TargetIteration: got %q, want \"5000\"", groups["TargetIteration"])
		} else {
			t.Logf("✓ TargetIteration = 5000")
		}
	})
}
