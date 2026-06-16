/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cdhandlers

import (
	"strings"
	"testing"
)

// TestComputeUnifiedDiffNoChange verifies identical content yields an empty diff.
func TestComputeUnifiedDiffNoChange(t *testing.T) {
	if ComputeUnifiedDiff("same", "same", "a", "b") != "" {
		t.Error("identical content should produce empty diff")
	}
}

// TestComputeUnifiedDiffWithChange verifies changes are reflected in the unified diff.
func TestComputeUnifiedDiffWithChange(t *testing.T) {
	diff := ComputeUnifiedDiff("line1\nline2\n", "line1\nline3\n", "old.yaml", "new.yaml")
	if diff == "" {
		t.Fatal("expected a non-empty diff for changed content")
	}
	if !strings.Contains(diff, "old.yaml") || !strings.Contains(diff, "new.yaml") {
		t.Error("diff should contain the from/to labels")
	}
	if !strings.Contains(diff, "-line2") || !strings.Contains(diff, "+line3") {
		t.Errorf("diff should show added/removed lines, got:\n%s", diff)
	}
}
