/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package opensearch

import (
	"testing"
	"time"

	"gotest.tools/assert"
)

func TestQueryIndex(t *testing.T) {
	endTime, err := time.Parse("2006-01-02T15:04:05", "2025-01-01T10:00:00")
	assert.NilError(t, err)

	client := &SearchClient{
		SearchClientConfig: SearchClientConfig{
			DefaultIndex: "node-",
		},
	}
	tests := []struct {
		name      string
		SinceTime time.Time
		UntilTime time.Time
		result    string
	}{
		{
			"across 2 months",
			endTime.Add(-time.Hour * 24 * 31),
			endTime,
			client.DefaultIndex + "*",
		},
		{
			"across 2 days",
			endTime.Add(-time.Hour * 24 * 2),
			endTime,
			client.DefaultIndex + "2024.12.30" + "," + client.DefaultIndex + "2024.12.31" + "," + client.DefaultIndex + "2025.01.01",
		},
		{
			"within the same day",
			endTime.Add(-time.Hour * 2),
			endTime,
			client.DefaultIndex + "2025.01.01",
		},
		{
			"with the same time",
			endTime,
			endTime,
			client.DefaultIndex + "2025.01.01",
		},
		{
			"across 0 o'clock",
			endTime.Add(-time.Hour * 18),
			endTime.Add(-time.Hour * 10).Add(time.Minute),
			client.DefaultIndex + "2024.12.31" + "," + client.DefaultIndex + "2025.01.01",
		},
		{
			"at 0 o'clock",
			endTime.Add(-time.Hour * 18),
			endTime.Add(-time.Hour * 10),
			client.DefaultIndex + "2024.12.31" + "," + client.DefaultIndex + "2025.01.01",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := client.generateIndexPattern(client.DefaultIndex, test.SinceTime, test.UntilTime)
			assert.NilError(t, err)
			assert.Equal(t, result, test.result)
		})
	}
}
