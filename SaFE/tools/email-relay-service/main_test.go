package main

import "testing"

// TestInitLogger verifies logger initialization across all supported levels.
func TestInitLogger(t *testing.T) {
	for _, level := range []string{"debug", "warn", "error", "info", "unknown"} {
		initLogger(level)
	}
}
