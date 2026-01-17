/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		name     string
		size     int64
		expected string
	}{
		{
			name:     "bytes",
			size:     512,
			expected: "512 B",
		},
		{
			name:     "kilobytes",
			size:     1024,
			expected: "1.00 KB",
		},
		{
			name:     "kilobytes with decimals",
			size:     2560,
			expected: "2.50 KB",
		},
		{
			name:     "megabytes",
			size:     1024 * 1024,
			expected: "1.00 MB",
		},
		{
			name:     "megabytes with decimals",
			size:     1536 * 1024,
			expected: "1.50 MB",
		},
		{
			name:     "gigabytes",
			size:     1024 * 1024 * 1024,
			expected: "1.00 GB",
		},
		{
			name:     "terabytes",
			size:     1024 * 1024 * 1024 * 1024,
			expected: "1.00 TB",
		},
		{
			name:     "zero bytes",
			size:     0,
			expected: "0 B",
		},
		{
			name:     "large file 50GB",
			size:     50 * 1024 * 1024 * 1024,
			expected: "50.00 GB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatFileSize(tt.size)
			assert.Equal(t, tt.expected, result)
		})
	}
}
