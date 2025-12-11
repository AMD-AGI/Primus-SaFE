package logs

import (
	"regexp"
	"testing"
)

// TestSimpleRegexParts tests each part of the regular expression step by step
func TestSimpleRegexParts(t *testing.T) {
	// Cleaned log
	log := `[20251202 09:12:08][rank-7/8][INFO ] [--------trainer.py:2560] : iteration 126/ 5000 | consumed samples: 16128 | elapsed time per iteration (ms): 13372.8/13364.7 | hip mem usage/free/total/usage_ratio: 153.81GB/102.17GB/255.98GB/60.09% | throughput per GPU (TFLOP/s/GPU): 567.6/568.0 | tokens per GPU (tokens/s/GPU): 9801.4/9807.3 | learning rate: 9.984820E-06 | global batch size: 128 | lm loss: 6.548988E-03 | loss scale: 1.0 | grad norm: 0.061 | number of skipped iterations: 0 | number of nan iterations: 0 |`

	tests := []struct {
		name    string
		pattern string
		expect  map[string]string
	}{
		{
			name:    "Simplest iteration match",
			pattern: `iteration\s+(\d+)`,
			expect: map[string]string{
				"": "iteration 126",
			},
		},
		{
			name:    "Iteration with named group",
			pattern: `iteration\s+(?P<CurrentIteration>\d+)`,
			expect: map[string]string{
				"CurrentIteration": "126",
			},
		},
		{
			name:    "Complete iteration+target",
			pattern: `iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)`,
			expect: map[string]string{
				"CurrentIteration": "126",
				"TargetIteration":  "5000",
			},
		},
		{
			name:    "Match iteration starting with .*",
			pattern: `.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)`,
			expect: map[string]string{
				"CurrentIteration": "126",
				"TargetIteration":  "5000",
			},
		},
		{
			name:    "Match consumed samples",
			pattern: `consumed samples:\s+(?P<ConsumedSamples>\d+)`,
			expect: map[string]string{
				"ConsumedSamples": "16128",
			},
		},
		{
			name:    "Match elapsed time",
			pattern: `elapsed\stime\sper\siteration\s\(ms\):\s+(?P<ElapsedTimePerIterationMS>\d+(?:\.\d+)*)`,
			expect: map[string]string{
				"ElapsedTimePerIterationMS": "13372.8",
			},
		},
		{
			name:    "Match HIP memory (full path)",
			pattern: `hip\s+mem\s+usage/free/total/usage_ratio:\s+(?P<MemUsage>\d+\.\d+)GB/(?P<MemFree>\d+\.\d+)GB/(?P<MemTotal>\d+\.\d+)GB/(?P<MemUsageRatio>\d+\.\d+)%`,
			expect: map[string]string{
				"MemUsage":      "153.81",
				"MemFree":       "102.17",
				"MemTotal":      "255.98",
				"MemUsageRatio": "60.09",
			},
		},
		{
			name:    "Match learning rate (scientific notation)",
			pattern: `learning\s+rate:\s+(?P<LearningRate>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)`,
			expect: map[string]string{
				"LearningRate": "9.984820E-06",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)

			if !re.MatchString(log) {
				t.Errorf("Pattern does NOT match")
				return
			}

			matches := re.FindStringSubmatch(log)
			if matches == nil {
				t.Errorf("FindStringSubmatch returned nil")
				return
			}

			// Extract named groups
			groups := make(map[string]string)
			names := re.SubexpNames()
			for i, name := range names {
				if i > 0 && i < len(matches) && name != "" {
					groups[name] = matches[i]
				}
			}

			t.Logf("Matches count: %d", len(matches))
			t.Logf("Full match: %s", matches[0])

			// Validate expected groups
			for expectedName, expectedValue := range tt.expect {
				if expectedName == "" {
					// Unnamed capture group, skip
					continue
				}
				got, ok := groups[expectedName]
				if !ok {
					t.Errorf("❌ Group %s not found", expectedName)
				} else if got != expectedValue {
					t.Errorf("❌ Group %s: got %q, want %q", expectedName, got, expectedValue)
				} else {
					t.Logf("✓ Group %s = %s", expectedName, got)
				}
			}
		})
	}
}
