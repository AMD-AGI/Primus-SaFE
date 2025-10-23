/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package timeutil

import (
	"fmt"
	"strconv"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	TimeRFC3339Short = "2006-01-02T15:04:05"
	TimeRFC3339Milli = "2006-01-02T15:04:05.999Z"
)

// FormatRFC3339 converts a *time.Time to its string representation in RFC3339Short format.
// Returns an empty string if the pointer is nil or points to a zero time value.
func FormatRFC3339(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format(TimeRFC3339Short)
}

// CvtStrUnixToTime converts a Unix timestamp string to time.Time
func CvtStrUnixToTime(strTime string) time.Time {
	if strTime == "" {
		return time.Time{}
	}
	intTime, err := strconv.ParseInt(strTime, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(intTime, 0).UTC()
}

// CvtTime3339ToCronStandard converts a time string in RFC3339 short format ("2006-01-02T15:04:05.000Z")
// to cron standard schedule format ("minute hour day month *"), ignoring the date year
// Returns an error if the input time string cannot be parsed.
func CvtTime3339ToCronStandard(timeStr string) (string, time.Time, error) {
	t, err := time.Parse(TimeRFC3339Milli, timeStr)
	if err != nil {
		return "", time.Time{}, err
	}
	scheduleStr := fmt.Sprintf("%d %d %d %d *", t.Minute(), t.Hour(), t.Day(), t.Month())
	return scheduleStr, t, nil
}

// CvtTimeOnlyToCronStandard converts a time-only string("15:04:05") to cron schedule format (minute hour * * *)
func CvtTimeOnlyToCronStandard(timeStr string) (string, time.Time, error) {
	t, err := time.Parse(time.TimeOnly, timeStr)
	if err != nil {
		return "", time.Time{}, err
	}
	scheduleStr := fmt.Sprintf("%d %d * * *", t.Minute(), t.Hour())
	return scheduleStr, t, nil
}

// CvtStrToRFC3339Milli converts a RFC3339 millisecond format string to UTC time.Time
func CvtStrToRFC3339Milli(timeStr string) (time.Time, error) {
	if timeStr == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(TimeRFC3339Milli, timeStr)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

// ParseCronStandard parses a cron schedule string
func ParseCronStandard(cronStandardSpec string) (cron.Schedule, error) {
	if cronStandardSpec == "" {
		return nil, fmt.Errorf("invalid input")
	}
	schedule, err := cron.ParseStandard(cronStandardSpec)
	if err != nil {
		return nil, err
	}
	return schedule, nil
}
