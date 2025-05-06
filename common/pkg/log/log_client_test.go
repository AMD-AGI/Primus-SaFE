/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package log

import (
	"testing"
	"time"

	"gotest.tools/assert"
)

func TestBuildIndex(t *testing.T) {
	endTime, err := time.Parse("2006-01-02T15:04:05", "2024-01-01T10:00:00")
	assert.NilError(t, err)

	client := &LogClient{
		prefix: "node-",
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
			client.prefix + "*",
		},
		{
			"across 2 days",
			endTime.Add(-time.Hour * 24 * 2),
			endTime,
			client.prefix + "2023.12.30" + "," + client.prefix + "2023.12.31" + "," + client.prefix + "2024.01.01",
		},
		{
			"within the same day",
			endTime.Add(-time.Hour * 2),
			endTime,
			client.prefix + "2024.01.01",
		},
		{
			"with the same time",
			endTime,
			endTime,
			client.prefix + "2024.01.01",
		},
		{
			"across 0 o'clock",
			endTime.Add(-time.Hour * 18),
			endTime.Add(-time.Hour * 10).Add(time.Minute),
			client.prefix + "2023.12.31" + "," + client.prefix + "2024.01.01",
		},
		{
			"at 0 o'clock",
			endTime.Add(-time.Hour * 18),
			endTime.Add(-time.Hour * 10),
			client.prefix + "2023.12.31" + "," + client.prefix + "2024.01.01",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := client.buildIndex(test.SinceTime, test.UntilTime)
			assert.NilError(t, err)
			assert.Equal(t, result, test.result)
		})
	}
}
