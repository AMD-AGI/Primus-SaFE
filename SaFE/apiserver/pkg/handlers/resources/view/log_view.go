/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import (
	"time"
)

const (
	DocId = "docId"
)

// The query input by the user for searching logs
type ListLogInput struct {
	// Start timestamp of the query (RFC3339 with milliseconds),
	// default depends on workload creation time or last 7 days.
	// e.g.: '2006-01-02T15:04:05.000Z'
	Since string `json:"since,omitempty"`
	// End timestamp of the query (RFC3339 with milliseconds), default now
	Until string `json:"until,omitempty"`
	// Pagination offset, default 0; must be >= 0 and < max docs(10000)
	Offset int `json:"offset,omitempty"`
	// Page size, default 100; constrained by max docs-per-query
	Limit int `json:"limit,omitempty"`
	// Time sort order: asc/desc; default asc
	Order string `json:"order,omitempty"`
	// Search for the given keywords. And Search. e.g. ["key1", "key2"]
	Keywords []string `json:"keywords,omitempty"`
	// Filter by pod names (comma-separated, OR filter)
	PodNames string `json:"podNames,omitempty"`
	// Filter by workload dispatch/run number; 0 means all
	DispatchCount int `json:"dispatchCount,omitempty"`
	// Filter by node names (comma-separated, OR filter); ignored if podNames is set
	NodeNames string `json:"nodeNames,omitempty"`
}

// For internal use, the request for searching logs
type ListLogRequest struct {
	ListLogInput
	// Start timestamp of the query
	SinceTime time.Time
	// End timestamp of the query
	UntilTime time.Time
	// All fields used for filtering.
	Filters        map[string]string
	IsEventRequest bool
}

// For internal use, the request for searching log context
type ListContextLogRequest struct {
	Query *ListLogRequest
	// For internal use, location to store the result.
	Id int
	// The maximum number of returned results
	Limit int
	// Input the specified document ID to retrieve the log context of that document.
	DocId string
}
