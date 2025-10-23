/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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
	schedule, err := ParseCronString(expr)
	assert.NilError(t, err)
	testTime, err := time.Parse(time.DateTime, "2024-03-08 01:01:09")
	assert.NilError(t, err)
	nextTime := schedule.Next(testTime)
	assert.Equal(t, nextTime.Format(time.DateTime), "2024-03-08 01:02:39")
	assert.Equal(t, nextTime.Sub(testTime).Seconds(), float64(90))

	expr = "0 1 23 10 *"
	schedule, err = ParseCronString(expr)
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

func TestCvtTimeOnlyToCronStandard(t *testing.T) {
	timeStr := "03:42:00"
	scheduleStr, _, err := CvtTimeOnlyToCron(timeStr)
	assert.NilError(t, err)

	timeStr2, err := CvtCronToTime(scheduleStr)
	assert.NilError(t, err)
	assert.Equal(t, timeStr, timeStr2)
}

func TestCvtTime3339ToCronStandard(t *testing.T) {
	timeStr := "2025-09-30T16:04:00.000Z"
	scheduleStr, _, err := CvtTime3339ToCron(timeStr)
	assert.NilError(t, err)
	assert.Equal(t, scheduleStr, "4 16 30 9 *")
}

func TestCvtStrToRFC3339Milli(t *testing.T) {
	timeStr := "2025-08-18T09:41:01.950926221Z"
	time1, err := CvtStrToRFC3339Milli(timeStr)
	assert.NilError(t, err)

	timeStr = "2025-08-18T09:41:01.950Z"
	time2, err := CvtStrToRFC3339Milli(timeStr)
	assert.NilError(t, err)
	assert.Equal(t, time1.Unix(), time2.Unix())
}
