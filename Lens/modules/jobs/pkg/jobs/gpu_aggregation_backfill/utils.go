package gpu_aggregation_backfill

import (
	"time"
)

// generateAllHours generates all hours in the time range
func generateAllHours(startTime, endTime time.Time) []time.Time {
	hours := make([]time.Time, 0)

	// Start from the first hour
	current := startTime.Truncate(time.Hour)
	end := endTime.Truncate(time.Hour)

	for !current.After(end) {
		hours = append(hours, current)
		current = current.Add(time.Hour)
	}

	return hours
}
