/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"fmt"
	"time"
	_ "time/tzdata"
)

const dateLayout = "2006-01-02"

// resolveTimezone parses an IANA timezone name (e.g. "Asia/Shanghai",
// "America/New_York") into a *time.Location. Returns UTC if tz is empty.
func resolveTimezone(tz string) (*time.Location, error) {
	if tz == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %q: %w", tz, err)
	}
	return loc, nil
}

// expandDateRangeForTimezone widens a user-local date range by ±1 day so that
// LiteLLM (which stores/filters timestamps in UTC) returns all records that
// could fall within the user's local date range.
//
// If loc is UTC, the dates are returned unchanged (no offset to compensate).
//
// Example (UTC+8): user selects 2026-03-20 ~ 2026-03-20
//
//	Beijing 03-20 = UTC 03-19 16:00 ~ UTC 03-20 16:00
//	→ query LiteLLM with start=03-19, end=03-21 to cover the full span.
func expandDateRangeForTimezone(startDate, endDate string, loc *time.Location) (adjStart, adjEnd string) {
	if loc == time.UTC {
		return startDate, endDate
	}

	start, err := time.Parse(dateLayout, startDate)
	if err != nil {
		return startDate, endDate
	}
	end, err := time.Parse(dateLayout, endDate)
	if err != nil {
		return startDate, endDate
	}
	return start.AddDate(0, 0, -1).Format(dateLayout),
		end.AddDate(0, 0, 1).Format(dateLayout)
}

// filterLogsByLocalDate keeps only the spend log entries whose StartTime,
// when converted to loc, falls within [startDate, endDate] (both inclusive).
// Entries with unparseable timestamps are kept to avoid data loss.
// If loc is UTC, the filter still works correctly (no conversion needed).
func filterLogsByLocalDate(logs []SpendLogEntry, startDate, endDate string, loc *time.Location) []SpendLogEntry {
	start, err := time.Parse(dateLayout, startDate)
	if err != nil {
		return logs
	}
	end, err := time.Parse(dateLayout, endDate)
	if err != nil {
		return logs
	}
	endExclusive := end.AddDate(0, 0, 1)

	filtered := make([]SpendLogEntry, 0, len(logs))
	for i := range logs {
		t := parseTimestamp(logs[i].StartTime)
		if t.IsZero() {
			filtered = append(filtered, logs[i])
			continue
		}
		local := t.In(loc)
		localDate := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
		if !localDate.Before(start) && localDate.Before(endExclusive) {
			filtered = append(filtered, logs[i])
		}
	}
	return filtered
}

// parseTimestamp tries common timestamp formats returned by LiteLLM's spend logs.
func parseTimestamp(s string) time.Time {
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000000",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.000000",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
