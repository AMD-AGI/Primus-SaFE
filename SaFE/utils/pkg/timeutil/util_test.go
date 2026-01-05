/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package timeutil

import (
	"fmt"
	"testing"
	"time"

	"gotest.tools/assert"
)

func TestParseSchedule(t *testing.T) {
	expr := "@every 90s"
	schedule, err := ParseCronStandard(expr)
	assert.NilError(t, err)
	testTime, err := time.Parse(time.DateTime, "2024-03-08 01:01:09")
	assert.NilError(t, err)
	nextTime := schedule.Next(testTime)
	assert.Equal(t, nextTime.Format(time.DateTime), "2024-03-08 01:02:39")
	assert.Equal(t, nextTime.Sub(testTime).Seconds(), float64(90))

	expr = "0 1 23 10 *"
	schedule, err = ParseCronStandard(expr)
	assert.NilError(t, err)
	now := time.Now()
	testTime, err = time.Parse(time.DateTime, fmt.Sprintf("%d-10-22 00:00:00", now.Year()))
	assert.NilError(t, err)
	nextTime = schedule.Next(testTime)
	assert.Equal(t, nextTime.Format(time.DateTime), fmt.Sprintf("%d-10-23 01:00:00", now.Year()))

	testTime, err = time.Parse(time.DateTime, fmt.Sprintf("%d-10-24 00:00:00", now.Year()))
	assert.NilError(t, err)
	nextTime = schedule.Next(testTime)
	assert.Equal(t, nextTime.Format(time.DateTime), fmt.Sprintf("%d-10-23 01:00:00", now.Year()+1))
}

func TestCvtTime3339ToCronStandard(t *testing.T) {
	timeStr := "2025-09-30T16:04:00.000Z"
	scheduleStr, scheduleTime, err := CvtTime3339ToCronStandard(timeStr)
	assert.NilError(t, err)
	assert.Equal(t, scheduleStr, "4 16 30 9 *")
	assert.Equal(t, FormatRFC3339(scheduleTime), "2025-09-30T16:04:00")
}

func TestCvtStrToRFC3339Milli(t *testing.T) {
	timeStr := "2025-08-18T09:41:01.950926221Z"
	time1, err := CvtStrToRFC3339Milli(timeStr)
	assert.NilError(t, err)

	timeStr = "2025-08-18T09:41:01.950Z"
	time2, err := CvtStrToRFC3339Milli(timeStr)
	assert.NilError(t, err)
	assert.Equal(t, time1.Unix(), time2.Unix())

	timeStr = "2025-08-18T09:41:01"
	time2, err = CvtStrToRFC3339Milli(timeStr)
	assert.NilError(t, err)
	assert.Equal(t, time1.Unix(), time2.Unix())
}

func TestFormatDuration(t *testing.T) {
	sec := 7500
	str := FormatDuration(int64(sec))
	assert.Equal(t, str, "2h5m")
	sec = 61
	str = FormatDuration(int64(sec))
	assert.Equal(t, str, "1m1s")
	sec = 3661
	str = FormatDuration(int64(sec))
	assert.Equal(t, str, "1h1m1s")
	sec = 0
	str = FormatDuration(int64(sec))
	assert.Equal(t, str, "0s")
}
