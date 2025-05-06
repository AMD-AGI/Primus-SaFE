/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package timeutil

import (
	"testing"
	"time"

	"gotest.tools/assert"
)

func TestParseSchedule(t *testing.T) {
	expr := "@every 30s"
	schedule, interval, err := ParseCronStandard(expr)
	assert.NilError(t, err)
	assert.Equal(t, interval, float64(30))

	expr = "@every 90s"
	schedule, _, err = ParseCronStandard(expr)
	assert.NilError(t, err)
	testTime, err := time.Parse(time.DateTime, "2024-03-08 01:01:09")
	assert.NilError(t, err)
	nextTime := schedule.Next(testTime)
	assert.Equal(t, nextTime.Format(time.DateTime), "2024-03-08 01:02:39")
	assert.Equal(t, nextTime.Sub(testTime).Seconds(), float64(90))

	schedule, interval, err = ParseCronStandard("10 3 * * *")
	assert.NilError(t, err)
	assert.Equal(t, interval, float64(11400))
}

func TestCvtTimeToCronStandard(t *testing.T) {
	timeStr := "03:42:00"
	scheduleStr, err := CvtTimeToCronStandard(timeStr)
	assert.NilError(t, err)

	timeStr2, err := CvtCronStandardToTime(scheduleStr)
	assert.NilError(t, err)
	assert.Equal(t, timeStr, timeStr2)
}
