/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package timeutil

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	TimeRFC3339Short = "2006-01-02T15:04:05"
	TimeRFC3339Milli = "2006-01-02T15:04:05.999Z"
)

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

func FormatRFC3339(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format(TimeRFC3339Short)
}

func CvtTimeToCronStandard(timeOnly string) (string, error) {
	t, err := time.Parse(time.TimeOnly, timeOnly)
	if err != nil {
		return "", err
	}
	scheduleStr := fmt.Sprintf("%d %d * * *", t.Minute(), t.Hour())
	return scheduleStr, nil
}

func CvtCronStandardToTime(scheduleStr string) (string, error) {
	values := strings.Split(scheduleStr, " ")
	if len(values) != 5 {
		return "", fmt.Errorf("invalid cron schedule")
	}
	return fmt.Sprintf("%02s:%02s:00", values[1], values[0]), nil
}

func ParseCronStandard(scheduleStr string) (cron.Schedule, float64, error) {
	if scheduleStr == "" {
		return nil, 0, fmt.Errorf("invalid input")
	}
	schedule, err := cron.ParseStandard(scheduleStr)
	if err != nil {
		return nil, 0, err
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(),
		0, 0, 0, 0, time.UTC)
	nextTime := schedule.Next(today.UTC())
	interval := nextTime.Sub(today).Seconds()
	return schedule, interval, nil
}

func CvtMilliSecToTime(milliseconds int64) time.Time {
	seconds := milliseconds / 1000
	nanoseconds := (milliseconds % 1000) * 1000000
	return time.Unix(seconds, nanoseconds).UTC()
}

func CvtStrToRFC3339Milli(timeStr string) (time.Time, error) {
	if timeStr == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(TimeRFC3339Milli, timeStr)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}
