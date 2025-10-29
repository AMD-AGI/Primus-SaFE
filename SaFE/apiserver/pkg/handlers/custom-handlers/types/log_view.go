/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	"time"
)

const (
	DocId = "docId"
)

// The query input by the user for searching logs
type ListLogInput struct {
	// Start timestamp of the query, default is one day ago. e.g.: '2006-01-02T15:04:05.000Z'
	Since string `json:"since,omitempty"`
	// End timestamp of the query, default is current time.
	Until string `json:"until,omitempty"`
	// Starting offset for the results, default is 0
	Offset int `json:"offset,omitempty"`
	// The maximum number of returned results, default is 100
	Limit int `json:"limit,omitempty"`
	// The sorting order. Valid values are "desc" (default) or "asc"
	Order string `json:"order,omitempty"`
	// Search for the given keywords. Multiple entries are treated as AND conditions.
	// Example: ["key1", "key2"]
	Keywords []string `json:"keywords,omitempty"`
	// Use the specified pod names for filtering.
	// Multiple entries are separated by commas, and treated as OR conditions
	PodNames string `json:"podNames,omitempty"`
	// Run number to filter by. Default 0 means all
	DispatchCount int `json:"dispatchCount,omitempty"`
	// Kubernetes node name used for filtering.
	// Multiple entries are separated by commas, and treated as OR conditions
	// If both PodNames and NodeNames are set, only PodNames will take effect.
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
	Filters map[string]string
}

// For internal use, the request for searching log context
type ListContextLogRequest struct {
	Query *ListLogRequest
	// For internal use, location to store the result.
	Id int
	// The maximum number of returned results
	Limit int
	// Input the specified document id to retrieve the log context of that document.
	DocId string
}
