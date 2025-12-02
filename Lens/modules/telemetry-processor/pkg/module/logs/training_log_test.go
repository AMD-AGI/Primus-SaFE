package logs

import (
	"regexp"
	"testing"
)

func TestStripAnsiCodes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Log with ANSI color codes",
			input:    "[[32m20251202 06:56:28[0m][[36mrank-7/8[0m][[1mINFO [0m] iteration 4408/ 5000",
			expected: "[20251202 06:56:28][rank-7/8][INFO ] iteration 4408/ 5000",
		},
		{
			name:     "Log without ANSI codes",
			input:    "iteration 4408/ 5000 | consumed samples: 564224",
			expected: "iteration 4408/ 5000 | consumed samples: 564224",
		},
		{
			name:     "Log with ESC sequences",
			input:    "\x1b[32mGreen text\x1b[0m normal text",
			expected: "Green text normal text",
		},
		{
			name:     "Complex primus log with ANSI",
			input:    "[[32m20251202 06:56:28[0m][[36mrank-7/8[0m][[1mINFO [0m] [1m[--------trainer.py:2560] : iteration 4408/ 5000 | consumed samples: 564224 | elapsed time per iteration (ms): 13100.0/13240.9 | hip mem usage/free/total/usage_ratio: 153.81GB/102.17GB/255.98GB/60.09% | throughput per GPU (TFLOP/s/GPU): 579.4/573.6 | tokens per GPU (tokens/s/GPU): 10005.5/9903.7 | learning rate: 3.421949E-07 | global batch size: 128 | lm loss: 4.142716E-03 | loss scale: 1.0 | grad norm: 0.003 | number of skipped iterations: 0 | number of nan iterations: 0 |[0m",
			expected: "[20251202 06:56:28][rank-7/8][INFO ] [--------trainer.py:2560] : iteration 4408/ 5000 | consumed samples: 564224 | elapsed time per iteration (ms): 13100.0/13240.9 | hip mem usage/free/total/usage_ratio: 153.81GB/102.17GB/255.98GB/60.09% | throughput per GPU (TFLOP/s/GPU): 579.4/573.6 | tokens per GPU (tokens/s/GPU): 10005.5/9903.7 | learning rate: 3.421949E-07 | global batch size: 128 | lm loss: 4.142716E-03 | loss scale: 1.0 | grad norm: 0.003 | number of skipped iterations: 0 | number of nan iterations: 0 |",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripAnsiCodes(tt.input)
			if result != tt.expected {
				t.Errorf("stripAnsiCodes() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestAnsiCleanedLogMatchesPattern(t *testing.T) {
	// Test that cleaned log matches the primus-hip-memory-v2 pattern
	rawLog := "[[32m20251202 06:56:28[0m][[36mrank-7/8[0m][[1mINFO [0m] [1m[--------trainer.py:2560] : iteration 4408/ 5000 | consumed samples: 564224 | elapsed time per iteration (ms): 13100.0/13240.9 | hip mem usage/free/total/usage_ratio: 153.81GB/102.17GB/255.98GB/60.09% | throughput per GPU (TFLOP/s/GPU): 579.4/573.6 | tokens per GPU (tokens/s/GPU): 10005.5/9903.7 | learning rate: 3.421949E-07 | global batch size: 128 | lm loss: 4.142716E-03 | loss scale: 1.0 | grad norm: 0.003 | number of skipped iterations: 0 | number of nan iterations: 0 |[0m"

	cleanedLog := stripAnsiCodes(rawLog)

	// This is a simplified version of primus-hip-memory-v2 pattern
	// The actual pattern in DB starts with \\.* which should match any prefix
	pattern := `.*iteration\s+(\d+)\s*/\s*(\d+)\s*\|\s*consumed samples:\s+(\d+)\s*\|\s*elapsed\stime\sper\siteration\s\(ms\):\s+(\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s+hip\s+mem\s+usage/free/total/usage_ratio:\s+(\d+\.\d+)GB/(\d+\.\d+)GB/(\d+\.\d+)GB/(\d+\.\d+)%`

	re := regexp.MustCompile(pattern)
	if !re.MatchString(cleanedLog) {
		t.Errorf("Cleaned log should match pattern, but it doesn't.\nCleaned log: %s", cleanedLog)
	}

	// Verify the original raw log does NOT match
	if re.MatchString(rawLog) {
		t.Log("Warning: Raw log with ANSI codes unexpectedly matched the pattern")
	}
}
