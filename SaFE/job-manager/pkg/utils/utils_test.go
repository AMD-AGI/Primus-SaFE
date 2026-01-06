/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"testing"

	"gotest.tools/assert"
)

func TestParseTorchFTSubIndex(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedIndex int
		expectedOk    bool
	}{
		{
			name:          "Valid name with index 0",
			input:         "my-job-0-abc12",
			expectedIndex: 0,
			expectedOk:    true,
		},
		{
			name:          "Valid name with index 2",
			input:         "my-job-2-xyz99",
			expectedIndex: 2,
			expectedOk:    true,
		},
		{
			name:          "Complex display name with dashes",
			input:         "complex-name-with-dashes-5-suffix",
			expectedIndex: 5,
			expectedOk:    true,
		},
		{
			name:          "Single word display name",
			input:         "job-3-abc",
			expectedIndex: 3,
			expectedOk:    true,
		},
		{
			name:          "No dashes - invalid",
			input:         "invalidname",
			expectedIndex: 0,
			expectedOk:    false,
		},
		{
			name:          "Only one dash - invalid",
			input:         "invalid-name",
			expectedIndex: 0,
			expectedOk:    false,
		},
		{
			name:          "Index is not a number - invalid",
			input:         "my-job-abc-suffix",
			expectedIndex: 0,
			expectedOk:    false,
		},
		{
			name:          "Empty string - invalid",
			input:         "",
			expectedIndex: 0,
			expectedOk:    false,
		},
		{
			name:          "Trailing dash - invalid index parse",
			input:         "my-job--suffix",
			expectedIndex: 0,
			expectedOk:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index, ok := ParseTorchFTGroupIndex(tt.input)
			assert.Equal(t, ok, tt.expectedOk, "ok mismatch for input: %s", tt.input)
			if tt.expectedOk {
				assert.Equal(t, index, tt.expectedIndex, "index mismatch for input: %s", tt.input)
			}
		})
	}
}
